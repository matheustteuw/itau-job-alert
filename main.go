// itau-job-alert monitora a listagem de vagas de Tecnologia do Itaú
// e envia um e-mail sempre que aparece uma vaga nova que bate com as
// palavras-chave configuradas (veja filter.go).
//
// Roda tanto localmente quanto como AWS Lambda:
//   - Local: go run .
//   - Lambda: o runtime seta a env var AWS_LAMBDA_FUNCTION_NAME automaticamente,
//     e o binário entra em lambda.Start(). Agendado via EventBridge (ex: a cada 15 min).
//
// Cada execução faz UMA checagem — não fica em loop, quem controla a
// frequência é o agendador (EventBridge no Lambda, cron/systemd localmente).
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/joho/godotenv"
)

const (
	// Página de vagas de Tecnologia do Itaú. O %d no final é o número da página.
	baseURL  = "https://carreiras.itau.com.br/%%C3%%A1rea/tecnologia-jobs/35299/8274976/%d"
	maxPages = 8 // teto de segurança, a listagem raramente passa disso
	seenFile = "seen_jobs.json"
)

type Job struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

func main() {
	// A Lambda seta essa env var automaticamente; localmente ela não existe.
	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		lambda.Start(handler)
		return
	}

	// Localmente, carrega o .env pro ambiente do processo (ausência do
	// arquivo não é erro — dá pra rodar só com env vars já exportadas).
	_ = godotenv.Load()

	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

// handler é o entrypoint chamado pelo runtime da Lambda a cada invocação
// (disparada pela regra do EventBridge).
func handler(ctx context.Context) error {
	return run(ctx)
}

// run faz uma checagem completa: busca vagas, filtra, compara com o histórico
// e manda e-mail se tiver vaga nova relevante.
func run(ctx context.Context) error {
	jobs, err := fetchAllTechJobs()
	if err != nil {
		return fmt.Errorf("erro ao buscar vagas: %w", err)
	}
	log.Printf("encontradas %d vagas de tecnologia na página do Itaú", len(jobs))

	keywords := loadKeywords()
	jobs = filterJobs(jobs, keywords)
	log.Printf("%d vaga(s) relevante(s) após filtro de palavras-chave", len(jobs))

	st, err := newStore(ctx)
	if err != nil {
		return fmt.Errorf("erro ao inicializar armazenamento do histórico: %w", err)
	}

	state, err := st.Load(ctx)
	if err != nil {
		return fmt.Errorf("erro ao carregar histórico: %w", err)
	}
	if state.Seen == nil {
		state.Seen = map[string]bool{}
	}

	var novas []Job
	for _, j := range jobs {
		if !state.Seen[j.ID] {
			novas = append(novas, j)
			state.Seen[j.ID] = true
		}
	}

	if len(novas) == 0 {
		if !shouldSendHeartbeat(state.LastHeartbeat, time.Now(), heartbeatInterval()) {
			log.Println("nenhuma vaga nova relevante desde a última checagem")
			return nil
		}

		log.Println("nenhuma vaga nova, mas passou do intervalo de heartbeat, enviando e-mail de status...")
		if err := sendHeartbeatEmail(len(state.Seen)); err != nil {
			return fmt.Errorf("erro ao enviar e-mail de heartbeat: %w", err)
		}
		state.LastHeartbeat = time.Now()
		if err := st.Save(ctx, state); err != nil {
			return fmt.Errorf("erro ao salvar histórico: %w", err)
		}
		log.Println("concluído")
		return nil
	}

	log.Printf("%d vaga(s) nova(s), enviando e-mail...", len(novas))
	if err := sendEmail(novas); err != nil {
		// Não salva o histórico se o e-mail falhou, pra tentar de novo na próxima execução
		return fmt.Errorf("erro ao enviar e-mail: %w", err)
	}

	// Um e-mail de vaga nova já mostra que o sistema está de pé, então conta como heartbeat.
	state.LastHeartbeat = time.Now()
	if err := st.Save(ctx, state); err != nil {
		return fmt.Errorf("erro ao salvar histórico: %w", err)
	}
	log.Println("concluído")
	return nil
}

// heartbeatInterval controla de quanto em quanto tempo, no máximo, o
// programa manda um e-mail de status quando não há vaga nova (só pra avisar
// "ainda estou rodando"). Configurável via HEARTBEAT_HOURS; default 24h.
func heartbeatInterval() time.Duration {
	raw := os.Getenv("HEARTBEAT_HOURS")
	if raw == "" {
		return 24 * time.Hour
	}
	hours, err := strconv.Atoi(raw)
	if err != nil || hours <= 0 {
		return 24 * time.Hour
	}
	return time.Duration(hours) * time.Hour
}

// shouldSendHeartbeat decide se já passou tempo suficiente desde o último
// heartbeat pra mandar um novo e-mail de status.
func shouldSendHeartbeat(last, now time.Time, interval time.Duration) bool {
	return now.Sub(last) >= interval
}

// fetchAllTechJobs percorre as páginas da listagem de Tecnologia e retorna todas as vagas encontradas.
func fetchAllTechJobs() ([]Job, error) {
	var all []Job
	client := &http.Client{Timeout: 20 * time.Second}

	for page := 1; page <= maxPages; page++ {
		url := fmt.Sprintf(baseURL, page)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		// Um User-Agent "normal" evita bloqueios bobos de bot básico.
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; itau-job-alert/1.0; personal use)")

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		doc, err := goquery.NewDocumentFromReader(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		pageJobs := parseJobs(doc)
		if len(pageJobs) == 0 {
			// página vazia = acabaram as páginas
			break
		}
		all = append(all, pageJobs...)

		time.Sleep(1 * time.Second) // educado com o servidor deles
	}

	return all, nil
}

var idFromURL = regexp.MustCompile(`/vaga/[^/]+/[^/]+/\d+/(\d+)`)

// parseJobs extrai as vagas de uma página de listagem já carregada.
func parseJobs(doc *goquery.Document) []Job {
	var jobs []Job
	seenOnPage := map[string]bool{}

	doc.Find(`a[href*="/vaga/"]`).Each(func(_ int, s *goquery.Selection) {
		href, ok := s.Attr("href")
		if !ok {
			return
		}
		m := idFromURL.FindStringSubmatch(href)
		if m == nil {
			return
		}
		id := m[1]
		if seenOnPage[id] {
			return
		}
		seenOnPage[id] = true

		title := strings.TrimSpace(s.Find("h2, h3").First().Text())
		if title == "" {
			title = strings.TrimSpace(strings.Split(s.Text(), "\n")[0])
		}
		if !strings.HasPrefix(href, "http") {
			href = "https://carreiras.itau.com.br" + href
		}

		jobs = append(jobs, Job{ID: id, Title: title, URL: href})
	})

	return jobs
}

// sendEmail manda um único e-mail listando todas as vagas novas encontradas.
func sendEmail(jobs []Job) error {
	host := os.Getenv("SMTP_HOST")
	portStr := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASS")
	from := os.Getenv("EMAIL_FROM")
	to := os.Getenv("EMAIL_TO")

	if host == "" || portStr == "" || user == "" || pass == "" || from == "" || to == "" {
		return fmt.Errorf("faltam variáveis de ambiente de SMTP/e-mail (veja .env.example)")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("SMTP_PORT inválida: %v", err)
	}

	var body strings.Builder
	subject := fmt.Sprintf("[Itau Jobs] %d vaga(s) nova(s) de Tecnologia", len(jobs))
	body.WriteString(fmt.Sprintf("Vaga(s) nova(s) encontradas em %s:\n\n", time.Now().Format("02/01/2006 15:04")))
	for _, j := range jobs {
		body.WriteString(fmt.Sprintf("- %s\n  %s\n\n", j.Title, j.URL))
	}

	msg := []byte(
		"From: " + from + "\r\n" +
			"To: " + to + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: text/plain; charset=UTF-8\r\n" +
			"\r\n" + body.String())

	auth := smtp.PlainAuth("", user, pass, host)
	addr := fmt.Sprintf("%s:%d", host, port)
	return smtp.SendMail(addr, auth, from, []string{to}, msg)
}

// sendHeartbeatEmail manda um e-mail curto avisando que o programa está
// rodando normalmente, mesmo sem vaga nova pra reportar (útil pra saber que
// o agendamento não quebrou silenciosamente).
func sendHeartbeatEmail(totalVagasRastreadas int) error {
	host := os.Getenv("SMTP_HOST")
	portStr := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASS")
	from := os.Getenv("EMAIL_FROM")
	to := os.Getenv("EMAIL_TO")

	if host == "" || portStr == "" || user == "" || pass == "" || from == "" || to == "" {
		return fmt.Errorf("faltam variáveis de ambiente de SMTP/e-mail (veja .env.example)")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("SMTP_PORT inválida: %v", err)
	}

	subject := "[Itau Jobs] Ainda funcionando — nenhuma vaga nova"
	body := fmt.Sprintf(
		"Checagem de status em %s: o monitor de vagas continua rodando normalmente.\n\n"+
			"Nenhuma vaga nova relevante desde o último e-mail. %d vaga(s) já rastreada(s) no total.\n",
		time.Now().Format("02/01/2006 15:04"), totalVagasRastreadas)

	msg := []byte(
		"From: " + from + "\r\n" +
			"To: " + to + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: text/plain; charset=UTF-8\r\n" +
			"\r\n" + body)

	auth := smtp.PlainAuth("", user, pass, host)
	addr := fmt.Sprintf("%s:%d", host, port)
	return smtp.SendMail(addr, auth, from, []string{to}, msg)
}

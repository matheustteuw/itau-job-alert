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

	_ "time/tzdata"
)

const (
	baseURL  = "https://carreiras.itau.com.br/%%C3%%A1rea/tecnologia-jobs/35299/8274976/%d"
	maxPages = 8
	seenFile = "seen_jobs.json"
)

// brazilLocation é usado só pra exibir horário nos e-mails (Lambda roda em
// UTC por padrão). Embarcado via time/tzdata pra não depender do runtime
// provided.al2023 ter o banco de fusos horários instalado no sistema.
var brazilLocation = func() *time.Location {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return time.UTC
	}
	return loc
}()

// pluralSuffix retorna "s" se n != 1, "" caso contrário — concordância
// simples tipo "vaga"/"vagas", "nova"/"novas", "encontrada"/"encontradas".
func pluralSuffix(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

type Job struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Company string `json:"company"`
}

func main() {
	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		lambda.Start(handler)
		return
	}

	_ = godotenv.Load()

	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func handler(ctx context.Context) error {
	return run(ctx)
}

// run faz uma checagem completa: busca vagas, filtra, compara com o histórico
// e manda e-mail se tiver vaga nova relevante.
func run(ctx context.Context) error {
	itauJobs, err := fetchAllTechJobs()
	if err != nil {
		return fmt.Errorf("erro ao buscar vagas do Itaú: %w", err)
	}
	log.Printf("encontradas %d vagas de tecnologia na página do Itaú", len(itauJobs))

	picpayJobs, err := fetchPicPayJobs()
	if err != nil {
		return fmt.Errorf("erro ao buscar vagas do PicPay: %w", err)
	}
	log.Printf("encontradas %d vagas no PicPay", len(picpayJobs))

	btgJobs, err := fetchBTGJobs()
	if err != nil {
		return fmt.Errorf("erro ao buscar vagas do BTG Pactual: %w", err)
	}
	log.Printf("encontradas %d vagas no BTG Pactual", len(btgJobs))

	keywords := loadKeywords()
	excludeKeywords := loadExcludeKeywords()

	var jobs []Job
	jobs = append(jobs, filterJobs(itauJobs, keywords, excludeKeywords)...)
	jobs = append(jobs, filterJobs(picpayJobs, keywords, excludeKeywords)...)
	jobs = append(jobs, filterJobs(btgJobs, keywords, excludeKeywords)...)
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
		return fmt.Errorf("erro ao enviar e-mail: %w", err)
	}

	state.LastHeartbeat = time.Now()
	if err := st.Save(ctx, state); err != nil {
		return fmt.Errorf("erro ao salvar histórico: %w", err)
	}
	log.Println("concluído")
	return nil
}

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

func shouldSendHeartbeat(last, now time.Time, interval time.Duration) bool {
	return now.Sub(last) >= interval
}

func fetchAllTechJobs() ([]Job, error) {
	var all []Job
	client := &http.Client{Timeout: 20 * time.Second}

	for page := 1; page <= maxPages; page++ {
		url := fmt.Sprintf(baseURL, page)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
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
			break
		}
		all = append(all, pageJobs...)

		time.Sleep(1 * time.Second)
	}

	return all, nil
}

var idFromURL = regexp.MustCompile(`/vaga/[^/]+/[^/]+/\d+/(\d+)`)

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

		jobs = append(jobs, Job{ID: id, Title: title, URL: href, Company: "Itaú"})
	})

	return jobs
}

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
	s := pluralSuffix(len(jobs))
	subject := fmt.Sprintf("[Itau Jobs] %d vaga%s nova%s de Tecnologia", len(jobs), s, s)
	body.WriteString(fmt.Sprintf("Vaga%s nova%s encontrada%s em %s:\n\n", s, s, s, time.Now().In(brazilLocation).Format("02/01/2006 15:04")))
	for _, j := range jobs {
		body.WriteString(fmt.Sprintf("- [%s] %s\n  %s\n\n", j.Company, j.Title, j.URL))
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
	s := pluralSuffix(totalVagasRastreadas)
	body := fmt.Sprintf(
		"Checagem de status em %s: o monitor de vagas continua rodando normalmente.\n\n"+
			"Nenhuma vaga nova relevante desde o último e-mail. %d vaga%s já rastreada%s no total.\n",
		time.Now().In(brazilLocation).Format("02/01/2006 15:04"), totalVagasRastreadas, s, s)

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

package main

import (
	"os"
	"strings"
)

// Uma vaga só é considerada relevante se o título contiver pelo menos uma.
// "junior"/"pleno"/"senior" ficam de fora de propósito: são termos de nível,
// não de área, então batem com vaga de qualquer área (ex: "Analista de
// Mídia Pleno") e geram falso positivo pra quem só quer vaga de dev/eng.
var defaultKeywords = []string{
	"engenheiro",
	"engenharia",
	"desenvolvedor",
	"desenvolvimento",
	"programador",
	"software",
	"backend",
	"frontend",
	"fullstack",
	"full stack",
	".net",
	"dotnet",
	"c#",
	"java",
	"javascript",
	"typescript",
	"python",
	"golang",
	"react",
	"node",
	"angular",
	"devops",
	"cloud",
	"azure",
	"aws",
	"ia",
	"inteligencia artificial",
	"machine learning",
}

// defaultExcludeKeywords derruba vagas afirmativas exclusivas pra pessoas
// com deficiência, independente de baterem com defaultKeywords — tanto o
// Itaú ("Exclusiva para Pessoas com deficiência") quanto o PicPay
// ("Exclusiva PCD") marcam isso no próprio título da vaga.
var defaultExcludeKeywords = []string{
	"pcd",
	"deficiencia",
}

// defaultBTGKeywords é o filtro específico do BTG Pactual — mais restrito
// que defaultKeywords porque o board deles cobre a empresa inteira (não só
// Tecnologia), então aqui vale ser mais específico pra não trazer vaga de
// área totalmente diferente. Mesmo motivo de defaultKeywords não ter
// "junior"/"pleno"/"senior": são termos de nível, batem com vaga de
// qualquer área.
var defaultBTGKeywords = []string{
	"desenvolvedor",
	"engenheiro",
	".net",
	"c#",
}

// loadCSVEnv lê uma lista separada por vírgula da env var indicada. Se não
// estiver definida (ou vazia após o parse), usa fallback.
func loadCSVEnv(envVar string, fallback []string) []string {
	raw := os.Getenv(envVar)
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			values = append(values, p)
		}
	}
	if len(values) == 0 {
		return fallback
	}
	return values
}

// loadKeywords lê a lista de palavras-chave da variável de ambiente KEYWORDS
// (separadas por vírgula). Se não estiver definida, usa defaultKeywords.
func loadKeywords() []string {
	return loadCSVEnv("KEYWORDS", defaultKeywords)
}

// loadExcludeKeywords lê a lista de exclusão da env var EXCLUDE_KEYWORDS. Se
// não estiver definida, usa defaultExcludeKeywords.
func loadExcludeKeywords() []string {
	return loadCSVEnv("EXCLUDE_KEYWORDS", defaultExcludeKeywords)
}

// loadBTGKeywords lê o filtro específico do BTG da env var BTG_KEYWORDS. Se
// não estiver definida, usa defaultBTGKeywords.
func loadBTGKeywords() []string {
	return loadCSVEnv("BTG_KEYWORDS", defaultBTGKeywords)
}

// filterJobs mantém apenas as vagas cujo título casa com alguma keyword e
// não casa com nenhuma excludeKeyword (exclusão tem prioridade).
func filterJobs(jobs []Job, keywords, excludeKeywords []string) []Job {
	normKeywords := make([]string, len(keywords))
	for i, kw := range keywords {
		normKeywords[i] = normalize(kw)
	}
	normExclude := make([]string, len(excludeKeywords))
	for i, kw := range excludeKeywords {
		normExclude[i] = normalize(kw)
	}

	var out []Job
	for _, j := range jobs {
		title := normalize(j.Title)

		excluded := false
		for _, kw := range normExclude {
			if strings.Contains(title, kw) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		for _, kw := range normKeywords {
			if strings.Contains(title, kw) {
				out = append(out, j)
				break
			}
		}
	}
	return out
}

var accentReplacer = strings.NewReplacer(
	"á", "a", "à", "a", "ã", "a", "â", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "õ", "o", "ô", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c",
)

// normalize deixa o texto minúsculo e sem acentos, pra comparação de keywords
// não depender de "Júnior" vs "Junior" ou maiúsculas/minúsculas.
func normalize(s string) string {
	return accentReplacer.Replace(strings.ToLower(s))
}

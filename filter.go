package main

import (
	"os"
	"strings"
)

// defaultKeywords é usado quando a variável de ambiente KEYWORDS não é definida.
// Uma vaga só é considerada relevante se o título contiver pelo menos uma
// dessas palavras (comparação sem acento e sem diferenciar maiúsc/minúsc).
var defaultKeywords = []string{
	"engenheiro",
	"engenharia",
	"java",
	".net",
	"desenvolvedor",
	"junior",
	"pleno",
	"senior",
}

// loadKeywords lê a lista de palavras-chave da variável de ambiente KEYWORDS
// (separadas por vírgula). Se não estiver definida, usa defaultKeywords.
func loadKeywords() []string {
	raw := os.Getenv("KEYWORDS")
	if strings.TrimSpace(raw) == "" {
		return defaultKeywords
	}
	parts := strings.Split(raw, ",")
	keywords := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			keywords = append(keywords, p)
		}
	}
	if len(keywords) == 0 {
		return defaultKeywords
	}
	return keywords
}

// filterJobs mantém apenas as vagas cujo título casa com alguma keyword.
func filterJobs(jobs []Job, keywords []string) []Job {
	normKeywords := make([]string, len(keywords))
	for i, kw := range keywords {
		normKeywords[i] = normalize(kw)
	}

	var out []Job
	for _, j := range jobs {
		title := normalize(j.Title)
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

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// btgJobsURL é a API pública e oficialmente documentada do Greenhouse
// (developers.greenhouse.io/job-board.html) pro board da BTG Pactual — mais
// estável que o endpoint do PicPay, que não é pensado pra integração externa.
const btgJobsURL = "https://boards-api.greenhouse.io/v1/boards/btgpactual/jobs"

type btgResponse struct {
	Jobs []struct {
		ID          int64  `json:"id"`
		Title       string `json:"title"`
		AbsoluteURL string `json:"absolute_url"`
	} `json:"jobs"`
}

// fetchBTGJobs busca todas as vagas abertas do BTG Pactual (qualquer área —
// o filtro por palavra-chave, mais restrito que o das outras empresas, é
// aplicado depois via btgKeywords em run()).
func fetchBTGJobs() ([]Job, error) {
	client := &http.Client{Timeout: 20 * time.Second}

	req, err := http.NewRequest("GET", btgJobsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var parsed btgResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	var jobs []Job
	for _, j := range parsed.Jobs {
		jobs = append(jobs, Job{
			// Prefixado por consistência com o PicPay — evita colidir com
			// IDs de outras empresas no histórico (state.Seen).
			ID:      fmt.Sprintf("btg:%d", j.ID),
			Title:   j.Title,
			URL:     j.AbsoluteURL,
			Company: "BTG Pactual",
		})
	}
	return jobs, nil
}

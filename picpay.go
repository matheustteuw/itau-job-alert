package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// picpayJobsURL é o endpoint público (sem autenticação) do Oracle Cloud HCM
// que a própria página de carreiras do PicPay usa por trás dos panos. Não é
// uma API oficialmente documentada pra esse uso — se o PicPay trocar de ATS
// de novo (já saíram da Gupy em algum momento), isso quebra sem aviso, igual
// ao scraping de HTML do Itaú.
const picpayJobsURL = "https://epdm.fa.la1.oraclecloud.com/hcmRestApi/resources/latest/recruitingCEJobRequisitions?onlyData=true&expand=requisitionList.secondaryLocations&finder=findReqs;siteNumber=CX_1,limit=100,facetsList=LOCATIONS%3BWORK_LOCATIONS%3BWORKPLACE_TYPES%3BTITLES%3BCATEGORIES%3BORGANIZATIONS%3BPOSTING_DATES%3BFLEX_FIELDS"

const picpayJobURLFormat = "https://epdm.fa.la1.oraclecloud.com/hcmUI/CandidateExperience/pt-BR/sites/PicPay/job/%s"

type picpayResponse struct {
	Items []struct {
		RequisitionList []struct {
			ID    string `json:"Id"`
			Title string `json:"Title"`
		} `json:"requisitionList"`
	} `json:"items"`
}

// fetchPicPayJobs busca todas as vagas abertas do PicPay. Diferente do
// Itaú, aqui é um endpoint JSON — sem HTML pra parsear, sem paginação (o
// limit=100 na URL já cobre o volume atual de vagas de uma vez).
func fetchPicPayJobs() ([]Job, error) {
	client := &http.Client{Timeout: 20 * time.Second}

	req, err := http.NewRequest("GET", picpayJobsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var parsed picpayResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	if len(parsed.Items) == 0 {
		return nil, nil
	}

	var jobs []Job
	for _, r := range parsed.Items[0].RequisitionList {
		jobs = append(jobs, Job{
			// Prefixado pra nunca colidir com IDs de outras empresas no
			// histórico (state.Seen) — o Itaú mantém o ID cru, sem prefixo,
			// pra não invalidar o histórico já salvo em produção.
			ID:      "picpay:" + r.ID,
			Title:   r.Title,
			URL:     fmt.Sprintf(picpayJobURLFormat, r.ID),
			Company: "PicPay",
		})
	}
	return jobs, nil
}

package main

import "testing"

func TestFilterJobs(t *testing.T) {
	jobs := []Job{
		{ID: "1", Title: "Engenheiro de Software Sênior - Backend Java"},
		{ID: "2", Title: "Desenvolvedor(a) .NET Pleno"},
		{ID: "3", Title: "Analista de Dados Júnior"},
		{ID: "4", Title: "Estagiário de Marketing"},
		{ID: "5", Title: "Especialista em Engenharia de Dados"},
		{ID: "6", Title: "Product Owner"},
	}

	got := filterJobs(jobs, defaultKeywords, defaultExcludeKeywords)

	// "Analista de Dados Júnior" (3) não entra: "junior" sozinho não é
	// suficiente, tem que ter termo de área (engenheiro/engenharia/etc).
	wantIDs := map[string]bool{"1": true, "2": true, "5": true}
	if len(got) != len(wantIDs) {
		t.Fatalf("esperava %d vagas relevantes, veio %d: %+v", len(wantIDs), len(got), got)
	}
	for _, j := range got {
		if !wantIDs[j.ID] {
			t.Errorf("vaga %q (id=%s) não deveria ter passado no filtro", j.Title, j.ID)
		}
	}
}

func TestFilterJobsExcludesNonDevRolesWithSeniorityWords(t *testing.T) {
	jobs := []Job{
		{ID: "1", Title: "[PicPay] Analista de Mídia de Performance Pleno"},
		{ID: "2", Title: "[PicPay] Analista de Produtos Junior - [Invest]"},
		{ID: "3", Title: "[PicPay] Analista de Negócios Pleno | Projetos de Atendimento (CX)"},
		{ID: "4", Title: "[Itaú] Engenharia de Software Backend Sr | Tech Lead - Bancos clientes riscos contas e Core banking"},
		{ID: "5", Title: "[Itaú] Engenharia de Software Fullstack .Net/Angular Pleno | Atendimento Assistido"},
	}

	got := filterJobs(jobs, defaultKeywords, defaultExcludeKeywords)

	wantIDs := map[string]bool{"4": true, "5": true}
	if len(got) != len(wantIDs) {
		t.Fatalf("esperava %d vagas relevantes, veio %d: %+v", len(wantIDs), len(got), got)
	}
	for _, j := range got {
		if !wantIDs[j.ID] {
			t.Errorf("vaga %q (id=%s) não deveria ter passado no filtro (nível sem área)", j.Title, j.ID)
		}
	}
}

func TestFilterJobsExcludesPCD(t *testing.T) {
	jobs := []Job{
		{ID: "1", Title: "Engenharia de Software Backend Java/Python Pleno | Exclusiva para Pessoas com deficiência"},
		{ID: "2", Title: "Analista Relacionamento Cliente I | Exclusiva PCD"},
		{ID: "3", Title: "Engenheiro de Software Sênior - Backend Java"},
	}

	got := filterJobs(jobs, defaultKeywords, defaultExcludeKeywords)

	if len(got) != 1 || got[0].ID != "3" {
		t.Errorf("esperava só a vaga 3 (as PCD deveriam ser excluídas), veio %+v", got)
	}
}

func TestNormalizeStripsAccentsAndCase(t *testing.T) {
	cases := map[string]string{
		"Júnior":  "junior",
		"SÊNIOR":  "senior",
		".NET":    ".net",
		"Engenharia de Software": "engenharia de software",
	}
	for in, want := range cases {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

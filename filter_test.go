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

	wantIDs := map[string]bool{"1": true, "2": true, "3": true, "5": true}
	if len(got) != len(wantIDs) {
		t.Fatalf("esperava %d vagas relevantes, veio %d: %+v", len(wantIDs), len(got), got)
	}
	for _, j := range got {
		if !wantIDs[j.ID] {
			t.Errorf("vaga %q (id=%s) não deveria ter passado no filtro", j.Title, j.ID)
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

func TestFilterJobsBTGKeywordsAreMoreSpecific(t *testing.T) {
	jobs := []Job{
		{ID: "1", Title: "Desenvolvedor(a) de Software Pleno (.NET) | IT Trading BSM"},
		{ID: "2", Title: "Desenvolvedor Backend (C#) | IT Escrituração"},
		{ID: "3", Title: "Analista de Risco Sênior"}, // "senior" bate no filtro padrão, mas não no do BTG
		{ID: "4", Title: "Engenheiro de Dados Júnior"},
	}

	got := filterJobs(jobs, defaultBTGKeywords, defaultExcludeKeywords)

	wantIDs := map[string]bool{"1": true, "2": true, "4": true}
	if len(got) != len(wantIDs) {
		t.Fatalf("esperava %d vagas, veio %d: %+v", len(wantIDs), len(got), got)
	}
	for _, j := range got {
		if !wantIDs[j.ID] {
			t.Errorf("vaga %q (id=%s) não deveria bater com o filtro mais restrito do BTG", j.Title, j.ID)
		}
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

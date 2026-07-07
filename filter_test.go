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

	got := filterJobs(jobs, defaultKeywords)

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

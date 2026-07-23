package main

import "testing"

func TestDefaultEDNEURL(test *testing.T) {
	if defaultEDNEURL != "https://www2.correios.com.br/sistemas/edne/download/eDNE_Basico.zip" {
		test.Errorf("defaultEDNEURL = %q, want official Correios URL", defaultEDNEURL)
	}
}

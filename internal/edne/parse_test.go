package edne_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/whoisclebs/cep-seed/internal/edne"
)

func newParser() *edne.Parser {
	return edne.NewParser()
}

// --- Localidade ---

func TestParseLocalidade(test *testing.T) {
	ctx := context.Background()
	parser := newParser()

	// LOG_LOCALIDADE.TXT: id@UF@nome@CEP@situacao@tipo@alias@abrev@IBGE  (Fields=9)
	input := "12@AC@Marechal Thaumaturgo@69983000@0@M@@Mal Thaumaturgo@1200351"
	reader := strings.NewReader(input)
	records, err := parser.ParseFile(ctx, reader, edne.LayoutLocalidade, "LOG_LOCALIDADE.TXT")
	if err != nil {
		test.Fatalf("parse localidade: %v", err)
	}
	if len(records) != 1 {
		test.Fatalf("got %d records, want 1", len(records))
	}
	record := records[0]
	if record.CEP != "69983000" {
		test.Errorf("CEP = %q, want 69983000", record.CEP)
	}
	if record.UF != "AC" {
		test.Errorf("UF = %q, want AC", record.UF)
	}
	if record.Localidade != "Marechal Thaumaturgo" {
		test.Errorf("Localidade = %q", record.Localidade)
	}
	if record.LocalidadeID != "12" {
		test.Errorf("LocalidadeID = %q, want 12", record.LocalidadeID)
	}
	if record.IBGE != "1200351" {
		test.Errorf("IBGE = %q, want 1200351", record.IBGE)
	}
}

func TestParseLocalidade_EmptyCEP(test *testing.T) {
	ctx := context.Background()
	parser := newParser()

	// CEP field 3 is empty — valid for locality records (serves as reference only).
	input := "16@AC@Rio Branco@@1@M@@Rio Branco@1200401"
	reader := strings.NewReader(input)
	records, err := parser.ParseFile(ctx, reader, edne.LayoutLocalidade, "LOG_LOCALIDADE.TXT")
	if err != nil {
		test.Fatalf("parse localidade empty cep: %v", err)
	}
	if len(records) != 1 {
		test.Fatalf("got %d records", len(records))
	}
	if records[0].CEP != "" {
		test.Errorf("CEP = %q, want empty", records[0].CEP)
	}
	if records[0].LocalidadeID != "16" {
		test.Errorf("LocalidadeID = %q", records[0].LocalidadeID)
	}
	if records[0].IBGE != "1200401" {
		test.Errorf("IBGE = %q", records[0].IBGE)
	}
}

func TestParseLocalidade_Accented(test *testing.T) {
	ctx := context.Background()
	parser := newParser()

	// ISO-8859-1: S\xe3o Paulo where \xe3 = ã
	input := "3550308@SP@S\xe3o Paulo@01001000@1@M@@S\xe3o Paulo@3550308"
	reader := strings.NewReader(input)
	records, err := parser.ParseFile(ctx, reader, edne.LayoutLocalidade, "LOG_LOCALIDADE.TXT")
	if err != nil {
		test.Fatalf("parse accented: %v", err)
	}
	if len(records) != 1 {
		test.Fatalf("got %d records", len(records))
	}
	if records[0].Localidade != "São Paulo" {
		test.Errorf("Localidade = %q, want São Paulo", records[0].Localidade)
	}
}

func TestParseLocalidade_BOM(test *testing.T) {
	ctx := context.Background()
	parser := newParser()

	input := "\xef\xbb\xbf12@AC@Marechal Thaumaturgo@69983000@0@M@@Mal Thaumaturgo@1200351"
	reader := strings.NewReader(input)
	records, err := parser.ParseFile(ctx, reader, edne.LayoutLocalidade, "LOG_LOCALIDADE.TXT")
	if err != nil {
		test.Fatalf("parse with BOM: %v", err)
	}
	if len(records) != 1 {
		test.Fatalf("got %d records", len(records))
	}
}

func TestParseLocalidade_EmptyLines(test *testing.T) {
	ctx := context.Background()
	parser := newParser()

	input := "\n\n12@AC@Marechal Thaumaturgo@69983000@0@M@@Mal Thaumaturgo@1200351\n\n"
	reader := strings.NewReader(input)
	records, err := parser.ParseFile(ctx, reader, edne.LayoutLocalidade, "LOG_LOCALIDADE.TXT")
	if err != nil {
		test.Fatalf("parse with empty lines: %v", err)
	}
	if len(records) != 1 {
		test.Fatalf("got %d records, want 1", len(records))
	}
}

// --- Bairro ---

func TestParseBairro(test *testing.T) {
	ctx := context.Background()
	parser := newParser()

	// LOG_BAIRRO.TXT: id@UF@loc@nome@abrev  (Fields=5)
	input := "60626@AC@16@Vila Albert Sampaio@Vl Albert Sampaio"
	reader := strings.NewReader(input)
	records, err := parser.ParseFile(ctx, reader, edne.LayoutBairro, "LOG_BAIRRO.TXT")
	if err != nil {
		test.Fatalf("parse bairro: %v", err)
	}
	if len(records) != 1 {
		test.Fatalf("got %d records", len(records))
	}
	record := records[0]
	if record.BairroID != "60626" {
		test.Errorf("BairroID = %q, want 60626", record.BairroID)
	}
	if record.LocalidadeID != "16" {
		test.Errorf("LocalidadeID = %q, want 16", record.LocalidadeID)
	}
	if record.Bairro != "Vila Albert Sampaio" {
		test.Errorf("Bairro = %q", record.Bairro)
	}
}

func TestParseBairro_Accented(test *testing.T) {
	ctx := context.Background()
	parser := newParser()

	// S\xe9 = Sé
	input := "1@SP@3550308@S\xe9@Se"
	reader := strings.NewReader(input)
	records, err := parser.ParseFile(ctx, reader, edne.LayoutBairro, "LOG_BAIRRO.TXT")
	if err != nil {
		test.Fatalf("parse bairro accented: %v", err)
	}
	if len(records) != 1 || records[0].Bairro != "Sé" {
		test.Errorf("Bairro = %q, want Sé", records[0].Bairro)
	}
}

// --- Logradouro ---

func TestParseLogradouro(test *testing.T) {
	ctx := context.Background()
	parser := newParser()

	// LOG_LOGRADOURO_SP.TXT (11 fields): id@UF@loc@bairro@reservado@nome@complemento@CEP@tipo@indicador@abrev
	input := "1001235@SP@8912@14716@@Octaviano de Arruda Campos@- de 960/961 ao fim@14810227@Avenida@S@Av Octaviano de A Campos"
	reader := strings.NewReader(input)
	records, err := parser.ParseFile(ctx, reader, edne.LayoutLogradouro, "LOG_LOGRADOURO_SP.TXT")
	if err != nil {
		test.Fatalf("parse logradouro: %v", err)
	}
	if len(records) != 1 {
		test.Fatalf("got %d records", len(records))
	}
	record := records[0]
	if record.CEP != "14810227" {
		test.Errorf("CEP = %q, want 14810227", record.CEP)
	}
	if record.UF != "SP" {
		test.Errorf("UF = %q, want SP", record.UF)
	}
	if record.LocalidadeID != "8912" {
		test.Errorf("LocalidadeID = %q, want 8912", record.LocalidadeID)
	}
	if record.BairroID != "14716" {
		test.Errorf("BairroID = %q, want 14716", record.BairroID)
	}
	if record.Logradouro != "Octaviano de Arruda Campos" {
		test.Errorf("Logradouro = %q", record.Logradouro)
	}
	if record.LogradouroTipo != "Avenida" {
		test.Errorf("LogradouroTipo = %q", record.LogradouroTipo)
	}
	if record.Complemento != "- de 960/961 ao fim" {
		test.Errorf("Complemento = %q", record.Complemento)
	}
}

func TestParseLogradouro_Accented(test *testing.T) {
	ctx := context.Background()
	parser := newParser()

	// Pra\xe7a da S\xe9 = Praça da Sé
	input := "1@SP@3550308@1@@Pra\xe7a da S\xe9@lado \xedmpar@01001000@Rua@S@Pc. da Se"
	reader := strings.NewReader(input)
	records, err := parser.ParseFile(ctx, reader, edne.LayoutLogradouro, "LOG_LOGRADOURO_SP.TXT")
	if err != nil {
		test.Fatalf("parse accented logradouro: %v", err)
	}
	if len(records) != 1 {
		test.Fatalf("got %d records", len(records))
	}
	if records[0].Logradouro != "Praça da Sé" {
		test.Errorf("Logradouro = %q, want Praça da Sé", records[0].Logradouro)
	}
	if records[0].Complemento != "lado ímpar" {
		test.Errorf("Complemento = %q, want lado ímpar", records[0].Complemento)
	}
}

func TestParseLogradouro_HyphenatedCEP(test *testing.T) {
	ctx := context.Background()
	parser := newParser()

	// CEP at position 7 with hyphen should be normalized.
	input := "1@SP@3550308@1@@Praca da Se@@01001-000@Rua@S@Pc. da Se"
	reader := strings.NewReader(input)
	records, err := parser.ParseFile(ctx, reader, edne.LayoutLogradouro, "LOG_LOGRADOURO_SP.TXT")
	if err != nil {
		test.Fatalf("parse hyphenated cep: %v", err)
	}
	if len(records) != 1 || records[0].CEP != "01001000" {
		test.Errorf("CEP = %q, want 01001000", records[0].CEP)
	}
}

// --- Grande Usuario ---

func TestParseGrandeUsuario(test *testing.T) {
	ctx := context.Background()
	parser := newParser()

	// LOG_GRANDE_USUARIO.TXT (9 fields):
	// id@UF@seq@loc@num@nome@endereco@CEP@abrev
	input := "33087@AC@1@39331@@AC Acrel\xe2ndia Clique e Retire@Avenida Paran\xe1, 296 Clique e Retire Correios@69945959@AC A C Retire"
	reader := strings.NewReader(input)
	records, err := parser.ParseFile(ctx, reader, edne.LayoutGrandeUsuario, "LOG_GRANDE_USUARIO.TXT")
	if err != nil {
		test.Fatalf("parse grande usuario: %v", err)
	}
	if len(records) != 1 {
		test.Fatalf("got %d records", len(records))
	}
	record := records[0]
	if record.CEP != "69945959" {
		test.Errorf("CEP = %q, want 69945959", record.CEP)
	}
	if record.UF != "AC" {
		test.Errorf("UF = %q, want AC", record.UF)
	}
	if record.LocalidadeID != "39331" {
		test.Errorf("LocalidadeID = %q, want 39331", record.LocalidadeID)
	}
	if record.Nome != "AC Acrelândia Clique e Retire" {
		test.Errorf("Nome = %q", record.Nome)
	}
	if record.Endereco != "Avenida Paraná, 296 Clique e Retire Correios" {
		test.Errorf("Endereco = %q", record.Endereco)
	}
}

// --- CPC ---

func TestParseCPC(test *testing.T) {
	ctx := context.Background()
	parser := newParser()

	// LOG_CPC.TXT (6 fields): id@UF@loc@nome@endereco@CEP
	input := "1285@AL@158@Conjunto Mutir\xe3o@Quadra 1 n\xba 37 - Conj.Mutir\xe3o - Rio Largo@57100990"
	reader := strings.NewReader(input)
	records, err := parser.ParseFile(ctx, reader, edne.LayoutCPC, "LOG_CPC.TXT")
	if err != nil {
		test.Fatalf("parse cpc: %v", err)
	}
	if len(records) != 1 {
		test.Fatalf("got %d records", len(records))
	}
	record := records[0]
	if record.CEP != "57100990" {
		test.Errorf("CEP = %q, want 57100990", record.CEP)
	}
	if record.UF != "AL" {
		test.Errorf("UF = %q, want AL", record.UF)
	}
	if record.LocalidadeID != "158" {
		test.Errorf("LocalidadeID = %q, want 158", record.LocalidadeID)
	}
	if record.Nome != "Conjunto Mutirão" {
		test.Errorf("Nome = %q", record.Nome)
	}
	if record.Endereco != "Quadra 1 nº 37 - Conj.Mutirão - Rio Largo" {
		test.Errorf("Endereco = %q", record.Endereco)
	}
}

// --- Unidade Operacional ---

func TestParseUnidadeOperacional(test *testing.T) {
	ctx := context.Background()
	parser := newParser()

	// LOG_UNID_OPER.TXT (10 fields):
	// id@UF@seq@loc@num@nome@endereco@CEP@indicador@abrev
	input := "12036@AC@3@39327@@AC Brasil\xe9ia@Avenida Prefeito Rolando Moreira, 170@69932970@S@AC Brasil\xe9ia"
	reader := strings.NewReader(input)
	records, err := parser.ParseFile(ctx, reader, edne.LayoutUnidadeOperacional, "LOG_UNID_OPER.TXT")
	if err != nil {
		test.Fatalf("parse unidade operacional: %v", err)
	}
	if len(records) != 1 {
		test.Fatalf("got %d records", len(records))
	}
	record := records[0]
	if record.CEP != "69932970" {
		test.Errorf("CEP = %q, want 69932970", record.CEP)
	}
	if record.UF != "AC" {
		test.Errorf("UF = %q, want AC", record.UF)
	}
	if record.LocalidadeID != "39327" {
		test.Errorf("LocalidadeID = %q, want 39327", record.LocalidadeID)
	}
	if record.Nome != "AC Brasiléia" {
		test.Errorf("Nome = %q", record.Nome)
	}
	if record.Endereco != "Avenida Prefeito Rolando Moreira, 170" {
		test.Errorf("Endereco = %q", record.Endereco)
	}
}

// --- Error cases ---

func TestParse_FieldCountDrift(test *testing.T) {
	ctx := context.Background()
	parser := newParser()

	input := "1@SP@8912"
	reader := strings.NewReader(input)
	_, err := parser.ParseFile(ctx, reader, edne.LayoutLogradouro, "LOG_LOGRADOURO_SP.TXT")
	if err == nil {
		test.Fatal("expected error for insufficient fields")
	}
	var pe *edne.ParseError
	if !errors.As(err, &pe) {
		test.Fatalf("expected *edne.ParseError, got %T", err)
	}
	if pe.File != "LOG_LOGRADOURO_SP.TXT" {
		test.Errorf("ParseError.File = %q", pe.File)
	}
	if pe.Line != 1 {
		test.Errorf("ParseError.Line = %d", pe.Line)
	}
}

func TestParse_InvalidCEP(test *testing.T) {
	ctx := context.Background()
	parser := newParser()

	input := "1@SP@3550308@1@@Nome@@123@Rua@S@Abrev"
	reader := strings.NewReader(input)
	_, err := parser.ParseFile(ctx, reader, edne.LayoutLogradouro, "LOG_LOGRADOURO_SP.TXT")
	if err == nil {
		test.Fatal("expected error for invalid CEP")
	}
	var pe *edne.ParseError
	if !errors.As(err, &pe) {
		test.Fatalf("expected *edne.ParseError, got %T", err)
	}
	if pe.File != "LOG_LOGRADOURO_SP.TXT" {
		test.Errorf("ParseError.File = %q", pe.File)
	}
}

func TestParse_EmptyCEPLogradouro(test *testing.T) {
	ctx := context.Background()
	parser := newParser()

	input := "1@SP@3550308@1@@Nome@@@Rua@S@Abrev"
	reader := strings.NewReader(input)
	_, err := parser.ParseFile(ctx, reader, edne.LayoutLogradouro, "LOG_LOGRADOURO_SP.TXT")
	if err == nil {
		test.Fatal("expected error for empty CEP in logradouro")
	}
	var pe *edne.ParseError
	if !errors.As(err, &pe) {
		test.Fatalf("expected *edne.ParseError, got %T", err)
	}
}

func TestParse_ContextCancellation(test *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	parser := newParser()

	input := "1@SP@3550308@1@@Praca da Se@@01001000@Rua@S@Abrev\n"
	reader := strings.NewReader(strings.Repeat(input, 100))

	cancel() // Cancel before reading.
	_, err := parser.ParseFile(ctx, reader, edne.LayoutLogradouro, "LOG_LOGRADOURO_SP.TXT")
	if err == nil {
		test.Error("expected error for cancelled context")
	}
}

func TestParse_MissingReferences(test *testing.T) {
	ctx := context.Background()
	parser := newParser()

	// References arbitrary locality/bairro IDs that have no matching record.
	// Parser should still parse; reference validation is the importer's job.
	input := "1@SP@99999999@999@@Rua Inexistente@@01001000@Rua@S@Abrev"
	reader := strings.NewReader(input)
	records, err := parser.ParseFile(ctx, reader, edne.LayoutLogradouro, "LOG_LOGRADOURO_SP.TXT")
	if err != nil {
		test.Fatalf("parse with missing refs: %v", err)
	}
	if len(records) != 1 {
		test.Fatalf("got %d records", len(records))
	}
	if records[0].BairroID != "999" {
		test.Errorf("BairroID = %q", records[0].BairroID)
	}
	if records[0].LocalidadeID != "99999999" {
		test.Errorf("LocalidadeID = %q", records[0].LocalidadeID)
	}
}

func TestParse_NilReader(test *testing.T) {
	parser := newParser()
	_, err := parser.ParseFile(context.Background(), nil, edne.LayoutLocalidade, "TEST.TXT")
	if err == nil {
		test.Error("expected error for nil reader")
	}
}

// --- Layout lookup ---

func TestLayoutForFile(test *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantOK   bool
		wantName string
	}{
		{"logradouro SP", "LOG_LOGRADOURO_SP.TXT", true, "LOG_LOGRADOURO"},
		{"logradouro lowercase", "log_logradouro_sp.txt", true, "LOG_LOGRADOURO"},
		{"logradouro AC", "LOG_LOGRADOURO_AC.TXT", true, "LOG_LOGRADOURO"},
		{"logradouro RJ", "LOG_LOGRADOURO_RJ.TXT", true, "LOG_LOGRADOURO"},
		{"localidade", "LOG_LOCALIDADE.TXT", true, "LOG_LOCALIDADE"},
		{"bairro", "LOG_BAIRRO.TXT", true, "LOG_BAIRRO"},
		{"grande usuario", "LOG_GRANDE_USUARIO.TXT", true, "LOG_GRANDE_USUARIO"},
		{"unid oper", "LOG_UNID_OPER.TXT", true, "LOG_UNID_OPER"},
		{"cpc", "LOG_CPC.TXT", true, "LOG_CPC"},
		{"unknown", "SOME_OTHER.TXT", false, ""},
		{"old imaginary", "LOG_LOG_LOG.TXT", false, ""},
		{"old tmp", "LOG_LOG_TMP.TXT", false, ""},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(subtest *testing.T) {
			layout, found := edne.LayoutForFile(testCase.filename)
			if found != testCase.wantOK {
				subtest.Errorf("LayoutForFile(%q) = %v, want %v", testCase.filename, found, testCase.wantOK)
			}
			if found && layout.Name != testCase.wantName {
				subtest.Errorf("LayoutForFile(%q) name = %q, want %q", testCase.filename, layout.Name, testCase.wantName)
			}
		})
	}
}

// --- CEP normalization ---

func TestParse_HyphenatedCEPNormalization(test *testing.T) {
	ctx := context.Background()
	parser := newParser()

	testCases := []struct {
		name  string
		input string
	}{
		{"eight digits", "1@SP@3550308@1@@Rua A@@01001000@Rua@S@Abrev"},
		{"with hyphen", "1@SP@3550308@1@@Rua A@@01001-000@Rua@S@Abrev"},
	}
	for _, testCase := range testCases {
		test.Run(testCase.name, func(subtest *testing.T) {
			records, err := parser.ParseFile(ctx, strings.NewReader(testCase.input), edne.LayoutLogradouro, "LOG_LOGRADOURO_SP.TXT")
			if err != nil {
				subtest.Fatal(err)
			}
			if len(records) == 0 || records[0].CEP != "01001000" {
				subtest.Errorf("CEP = %q, want 01001000", records[0].CEP)
			}
		})
	}
}

func TestParse_InvalidCEPrejected(test *testing.T) {
	ctx := context.Background()
	parser := newParser()

	testCases := []struct {
		name  string
		input string
	}{
		{"too short", "1@SP@3550308@1@@Rua A@@123@Rua@S@Abrev"},
		{"empty", "1@SP@3550308@1@@Rua A@@@Rua@S@Abrev"},
		{"letters", "1@SP@3550308@1@@Rua A@@abcdefgh@Rua@S@Abrev"},
	}
	for _, testCase := range testCases {
		test.Run(testCase.name, func(subtest *testing.T) {
			_, err := parser.ParseFile(ctx, strings.NewReader(testCase.input), edne.LayoutLogradouro, "LOG_LOGRADOURO_SP.TXT")
			if err == nil {
				subtest.Error("expected error for invalid CEP")
			}
		})
	}
}

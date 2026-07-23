package edne

// SourceRecord holds the extracted fields from a single parsed eDNE source record.
// Fields not applicable to a record class are stored as empty strings.
type SourceRecord struct {
	Class          RecordClass
	CEP            string // normalized 8-digit CEP (no hyphen); empty for reference-only records
	UF             string // 2-letter state abbreviation
	Localidade     string // city/locality name (resolved during import)
	LocalidadeID   string // locality identifier for joins (field `loc` or LOCALIDADE.`id`)
	IBGE           string // 7-digit IBGE municipality code (from LOCALIDADE record)
	Bairro         string // neighborhood name (resolved during import)
	BairroID       string // neighborhood identifier for joins
	LogradouroTipo string // street type/prefix for LOG records, e.g. Rua, Praça
	Logradouro     string // street/address name (LOG class)
	Complemento    string // address complement (LOG class)
	Nome           string // entity/recipient/unit name (GRU/UNI/CPC)
	Endereco       string // address line (GRU/UNI/CPC)
	LineNumber     int
	FileName       string
}

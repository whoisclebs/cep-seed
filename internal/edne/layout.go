package edne

import (
	"fmt"
	"path"
	"strings"
)

// RecordClass identifies the type of eDNE record.
type RecordClass int

const (
	ClassLogradouro         RecordClass = iota + 1 // LOG — streets/addresses
	ClassLocalidade                                // LOC — localities (cities)
	ClassBairro                                    // BAI — neighborhoods
	ClassGrandeUsuario                             // GRU — large mail recipients
	ClassUnidadeOperacional                        // UNI — postal units
	ClassCPC                                       // CPC — community PO boxes
)

func (c RecordClass) String() string {
	switch c {
	case ClassLogradouro:
		return "logradouro"
	case ClassLocalidade:
		return "localidade"
	case ClassBairro:
		return "bairro"
	case ClassGrandeUsuario:
		return "grande_usuario"
	case ClassUnidadeOperacional:
		return "unidade_operacional"
	case ClassCPC:
		return "cpc"
	default:
		return fmt.Sprintf("unknown(%d)", c)
	}
}

// AddressType returns the uppercase English enum value for this class.
func (c RecordClass) AddressType() string {
	switch c {
	case ClassLogradouro:
		return "STREET"
	case ClassLocalidade:
		return "LOCALITY"
	case ClassGrandeUsuario:
		return "LARGE_USER"
	case ClassUnidadeOperacional:
		return "POSTAL_UNIT"
	case ClassCPC:
		return "COMMUNITY_POSTAL_BOX"
	default:
		return ""
	}
}

// Layout defines the expected structure of a single eDNE delimited record class.
type Layout struct {
	Class  RecordClass
	Name   string // canonical display name (e.g. "LOG_LOGRADOURO")
	Glob   string // path.Match glob for file discovery (e.g. "LOG_LOGRADOURO_*.TXT")
	Fields int    // expected number of delimited fields
	CEPPos int    // 0-indexed position of the CEP field (-1 if no CEP)
	Desc   string // human-readable description
}

// Real layouts confirmed against eDNE Básico release 26071 Delimitado/ files.
// Files use @ as delimiter, ISO-8859-1 encoding, CRLF line endings, no header.
// Fields are 0-indexed.
var (
	// LOG_LOGRADOURO_XX.TXT — 27 files (one per UF).
	// Fields: id0 UF1 loc2 bairro3 reservado4 nome5 complemento6 CEP7 tipo8 indicador9 abreviatura10
	LayoutLogradouro = Layout{
		Class: ClassLogradouro, Name: "LOG_LOGRADOURO", Glob: "LOG_LOGRADOURO_*.TXT",
		Fields: 11, CEPPos: 7,
		Desc: "LOG: id0 UF1 loc2 bairro3 reservado4 nome5 complemento6 CEP7 tipo8 indicador9 abrev10",
	}

	// LOG_LOCALIDADE.TXT
	// Fields: id0 UF1 nome2 CEP3 situacao4 tipo5 alias6 abreviatura7 IBGE8
	// NOTE: CEP may be empty; such records serve as reference for joins only.
	LayoutLocalidade = Layout{
		Class: ClassLocalidade, Name: "LOG_LOCALIDADE", Glob: "LOG_LOCALIDADE.TXT",
		Fields: 9, CEPPos: 3,
		Desc: "LOC: id0 UF1 nome2 CEP3 situacao4 tipo5 alias6 abrev7 IBGE8",
	}

	// LOG_BAIRRO.TXT
	// Fields: id0 UF1 loc2 nome3 abrev4
	// Bairros do not have their own CEP.
	LayoutBairro = Layout{
		Class: ClassBairro, Name: "LOG_BAIRRO", Glob: "LOG_BAIRRO.TXT",
		Fields: 5, CEPPos: -1,
		Desc: "BAI: id0 UF1 loc2 nome3 abrev4",
	}

	// LOG_GRANDE_USUARIO.TXT
	// Fields: id0 UF1 seq2 loc3 num4 nome5 endereco6 CEP7 abrev8
	LayoutGrandeUsuario = Layout{
		Class: ClassGrandeUsuario, Name: "LOG_GRANDE_USUARIO", Glob: "LOG_GRANDE_USUARIO.TXT",
		Fields: 9, CEPPos: 7,
		Desc: "GRU: id0 UF1 seq2 loc3 num4 nome5 endereco6 CEP7 abrev8",
	}

	// LOG_UNID_OPER.TXT
	// Fields: id0 UF1 seq2 loc3 num4 nome5 endereco6 CEP7 indicador8 abrev9
	LayoutUnidadeOperacional = Layout{
		Class: ClassUnidadeOperacional, Name: "LOG_UNID_OPER", Glob: "LOG_UNID_OPER.TXT",
		Fields: 10, CEPPos: 7,
		Desc: "UNI: id0 UF1 seq2 loc3 num4 nome5 endereco6 CEP7 indicador8 abrev9",
	}

	// LOG_CPC.TXT
	// Fields: id0 UF1 loc2 nome3 endereco4 CEP5
	LayoutCPC = Layout{
		Class: ClassCPC, Name: "LOG_CPC", Glob: "LOG_CPC.TXT",
		Fields: 6, CEPPos: 5,
		Desc: "CPC: id0 UF1 loc2 nome3 endereco4 CEP5",
	}

	// AllSupported collects every layout the importer recognises.
	// Order matters for glob matching: more specific patterns first.
	AllSupported = []Layout{
		LayoutLogradouro,
		LayoutLocalidade,
		LayoutBairro,
		LayoutGrandeUsuario,
		LayoutUnidadeOperacional,
		LayoutCPC,
	}
)

// LayoutForFile returns the first layout whose Glob matches the given filename (case-insensitive).
// It uses path.Match semantics: "LOG_LOGRADOURO_*.TXT" matches "LOG_LOGRADOURO_SP.TXT".
func LayoutForFile(name string) (Layout, bool) {
	upper := strings.ToUpper(name)
	for _, layout := range AllSupported {
		matched, err := path.Match(layout.Glob, upper)
		if err == nil && matched {
			return layout, true
		}
	}
	return Layout{}, false
}

package edne

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/whoisclebs/cep-seed/internal/apperror"
)

// ParseError is a typed error for parse failures with file/line/cause.
type ParseError struct {
	File  string
	Line  int
	Cause error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s line %d: %v", e.File, e.Line, e.Cause)
}

// Unwrap returns the underlying cause.
func (e *ParseError) Unwrap() error { return e.Cause }

// Parser reads and validates eDNE delimited files.
type Parser struct{}

// NewParser creates a new eDNE parser.
func NewParser() *Parser {
	return &Parser{}
}

// ParseFile reads a single eDNE delimited file and returns extracted records.
// It handles ISO-8859-1 to UTF-8 conversion, @-delimited field splitting,
// validates field counts according to the expected layout, and normalises
// CEP values where applicable. Records without CEPs (e.g. some LOCALIDADE
// entries) are still returned with an empty CEP field for reference use.
func (parser *Parser) ParseFile(ctx context.Context, reader io.Reader, layout Layout, fileName string) ([]SourceRecord, error) {
	if reader == nil {
		return nil, &ParseError{File: fileName, Line: 0, Cause: fmt.Errorf("nil reader")}
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 512*1024)

	var records []SourceRecord
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}

		// Skip BOM if present.
		if lineNum == 1 && len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF {
			raw = raw[3:]
		}

		// Decode ISO-8859-1 to UTF-8.
		decoded := decodeISO8859_1(raw)
		if !utf8.ValidString(decoded) {
			return nil, &ParseError{File: fileName, Line: lineNum, Cause: fmt.Errorf("invalid UTF-8 after decoding")}
		}

		// Parse @-delimited fields.
		fields := splitFields(decoded)
		if len(fields) == 0 {
			continue
		}

		// Validate field count.
		if layout.Fields > 0 && len(fields) < layout.Fields {
			return nil, &ParseError{File: fileName, Line: lineNum, Cause: fmt.Errorf("field count %d, expected at least %d (layout %s)", len(fields), layout.Fields, layout.Name)}
		}

		record, err := extractRecord(fields, layout, fileName, lineNum)
		if err != nil {
			return nil, err
		}

		records = append(records, record)
	}

	if err := scanner.Err(); err != nil {
		return nil, apperror.Wrapf(apperror.KindParse, err, "%s: scanner error", fileName)
	}

	return records, nil
}

// extractRecord converts raw @-delimited fields into a structured SourceRecord.
func extractRecord(fields []string, layout Layout, fileName string, lineNum int) (SourceRecord, error) {
	record := SourceRecord{
		Class:      layout.Class,
		LineNumber: lineNum,
		FileName:   fileName,
	}

	switch layout.Class {
	case ClassLocalidade:
		// id0 UF1 nome2 CEP3 situacao4 tipo5 alias6 abrev7 IBGE8
		record.LocalidadeID = strings.TrimSpace(fields[0])
		record.UF = strings.TrimSpace(fields[1])
		record.Localidade = strings.TrimSpace(fields[2])
		record.CEP = normalizeCEP(fields[3])
		if len(fields) > 8 {
			record.IBGE = strings.TrimSpace(fields[8])
		}

	case ClassBairro:
		// id0 UF1 loc2 nome3 abrev4
		record.BairroID = strings.TrimSpace(fields[0])
		record.UF = strings.TrimSpace(fields[1])
		record.LocalidadeID = strings.TrimSpace(fields[2])
		record.Bairro = strings.TrimSpace(fields[3])

	case ClassLogradouro:
		// id0 UF1 loc2 bairro3 reservado4 nome5 complemento6 CEP7 tipo8 indicador9 abrev10
		record.UF = strings.TrimSpace(fields[1])
		record.LocalidadeID = strings.TrimSpace(fields[2])
		record.BairroID = strings.TrimSpace(fields[3])
		record.LogradouroTipo = strings.TrimSpace(fields[8])
		record.Logradouro = strings.TrimSpace(fields[5])
		record.Complemento = strings.TrimSpace(fields[6])

		cep := normalizeCEP(fields[7])
		if cep == "" {
			return SourceRecord{}, &ParseError{File: fileName, Line: lineNum, Cause: fmt.Errorf("invalid CEP %q at position 7", fields[7])}
		}
		record.CEP = cep

	case ClassGrandeUsuario:
		// id0 UF1 seq2 loc3 num4 nome5 endereco6 CEP7 abrev8
		record.UF = strings.TrimSpace(fields[1])
		record.LocalidadeID = strings.TrimSpace(fields[3])
		record.Nome = strings.TrimSpace(fields[5])
		record.Endereco = strings.TrimSpace(fields[6])

		cep := normalizeCEP(fields[7])
		if cep == "" {
			return SourceRecord{}, &ParseError{File: fileName, Line: lineNum, Cause: fmt.Errorf("invalid CEP %q at position 7", fields[7])}
		}
		record.CEP = cep

	case ClassUnidadeOperacional:
		// id0 UF1 seq2 loc3 num4 nome5 endereco6 CEP7 indicador8 abrev9
		record.UF = strings.TrimSpace(fields[1])
		record.LocalidadeID = strings.TrimSpace(fields[3])
		record.Nome = strings.TrimSpace(fields[5])
		record.Endereco = strings.TrimSpace(fields[6])

		cep := normalizeCEP(fields[7])
		if cep == "" {
			return SourceRecord{}, &ParseError{File: fileName, Line: lineNum, Cause: fmt.Errorf("invalid CEP %q at position 7", fields[7])}
		}
		record.CEP = cep

	case ClassCPC:
		// id0 UF1 loc2 nome3 endereco4 CEP5
		record.UF = strings.TrimSpace(fields[1])
		record.LocalidadeID = strings.TrimSpace(fields[2])
		record.Nome = strings.TrimSpace(fields[3])
		record.Endereco = strings.TrimSpace(fields[4])

		cep := normalizeCEP(fields[5])
		if cep == "" {
			return SourceRecord{}, &ParseError{File: fileName, Line: lineNum, Cause: fmt.Errorf("invalid CEP %q at position 5", fields[5])}
		}
		record.CEP = cep
	}

	return record, nil
}

// normalizeCEP strips non-digit characters from the raw CEP input and
// returns exactly eight decimal digits. Returns empty string if the
// result is not exactly eight digits.
func normalizeCEP(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(8)
	for _, ch := range trimmed {
		if ch >= '0' && ch <= '9' {
			builder.WriteRune(ch)
		}
	}
	result := builder.String()
	if len(result) != 8 {
		return ""
	}
	return result
}

// splitFields splits a delimited line by @.
func splitFields(line string) []string {
	if line == "" {
		return nil
	}
	return strings.Split(line, "@")
}

// decodeISO8859_1 converts ISO-8859-1 bytes to UTF-8.
func decodeISO8859_1(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	raw = bytes.TrimRight(raw, "\r\n")

	runes := make([]rune, 0, len(raw))
	for _, byteValue := range raw {
		if byteValue == 0x0A || byteValue == 0x0D {
			continue
		}
		runes = append(runes, rune(byteValue))
	}
	return string(runes)
}

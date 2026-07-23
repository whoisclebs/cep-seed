// Package edne reads and validates the delimited eDNE Básico source files.
//
// eDNE Básico is Correios' official Brazilian address database. The delimited
// representation (Delimitado/ directory) uses ISO-8859-1 (Latin-1) encoding,
// @ as field separator, and CRLF line endings. No header row is present.
//
// Supported record classes (confirmed against release 26071):
//
//   - Logradouros (streets/addresses): LOG_LOGRADOURO_XX.TXT (27 files, one per UF)
//     11 fields @-delimited: id0 UF1 loc2 bairro3 reservado4 nome5 complemento6 CEP7 tipo8 indicador9 abrev10
//   - Localidades (localities): LOG_LOCALIDADE.TXT (9 fields)
//     id0 UF1 nome2 CEP3 situacao4 tipo5 alias6 abrev7 IBGE8 — CEP may be empty
//   - Bairros (neighborhoods): LOG_BAIRRO.TXT (5 fields)
//     id0 UF1 loc2 nome3 abrev4
//   - Grandes Usuários (large users/mail recipients): LOG_GRANDE_USUARIO.TXT (9 fields)
//     id0 UF1 seq2 loc3 num4 nome5 endereco6 CEP7 abrev8
//   - Unidades Operacionais (postal operational units): LOG_UNID_OPER.TXT (10 fields)
//     id0 UF1 seq2 loc3 num4 nome5 endereco6 CEP7 indicador8 abrev9
//   - Caixas Postais Comunitárias (community PO boxes): LOG_CPC.TXT (6 fields)
//     id0 UF1 loc2 nome3 endereco4 CEP5
//
// All files use @ as delimiter. ISO-8859-1 → UTF-8 conversion is handled
// automatically. Field positions are 0-indexed as documented above.
package edne

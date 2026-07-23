package importer

import (
	"strings"

	"github.com/whoisclebs/cep-seed/internal/edne"
)

// buildCanonical processes parsed file results into canonical rows,
// building reference maps for locality and bairro joins, detecting
// CEP collisions, and collecting unresolved reference warnings.
func buildCanonical(results []fileResult) (map[string]canonicalRow, []CollisionRecord, map[string]int, []UnresolvedReference, error) {
	localidades := buildLocalityMap(results)
	bairros := buildBairroMap(results)

	canonical := make(map[string]canonicalRow)
	var collisions []CollisionRecord
	var warnings []UnresolvedReference
	accepted := make(map[string]int)

	addRow := func(record edne.SourceRecord, addressType string) {
		if _, alreadyExists := canonical[record.CEP]; alreadyExists {
			collisions = append(collisions, CollisionRecord{
				CEP:         record.CEP,
				Existing:    canonical[record.CEP].addressType,
				Conflicting: addressType,
				Source:      record.FileName,
			})
			return
		}

		localityID, locWarning := resolveLocalityID(record, localidades)
		if locWarning != nil {
			warnings = append(warnings, *locWarning)
		}
		neighborhoodID, hoodWarning := resolveNeighborhoodID(record, bairros)
		if hoodWarning != nil {
			warnings = append(warnings, *hoodWarning)
		}
		street, additionalInfo := resolveAddress(record)
		postalUnit := resolvePostalUnit(record, addressType)

		canonical[record.CEP] = canonicalRow{
			postalCode:            record.CEP,
			street:                street,
			additionalInformation: additionalInfo,
			stateCode:             record.UF,
			localityID:            localityID,
			neighborhoodID:        neighborhoodID,
			addressType:           addressType,
			postalUnit:            postalUnit,
		}
	}

	for _, result := range results {
		addressType := result.layout.Class.AddressType()
		for _, record := range result.records {
			if record.CEP == "" {
				continue
			}
			addRow(record, addressType)
			accepted[addressType]++
		}
	}

	return canonical, collisions, accepted, warnings, nil
}

// buildLocalityMap indexes LOCALIDADE records by their ID for join resolution.
func buildLocalityMap(results []fileResult) map[string]edne.SourceRecord {
	localityMap := make(map[string]edne.SourceRecord)
	for _, result := range results {
		if result.layout.Class != edne.ClassLocalidade {
			continue
		}
		for _, record := range result.records {
			if record.LocalidadeID != "" {
				localityMap[record.LocalidadeID] = record
			}
		}
	}
	return localityMap
}

// buildBairroMap indexes BAIRRO records by their ID for join resolution.
func buildBairroMap(results []fileResult) map[string]edne.SourceRecord {
	neighborhoodMap := make(map[string]edne.SourceRecord)
	for _, result := range results {
		if result.layout.Class != edne.ClassBairro {
			continue
		}
		for _, record := range result.records {
			if record.BairroID != "" {
				neighborhoodMap[record.BairroID] = record
			}
		}
	}
	return neighborhoodMap
}

// resolveLocalityID resolves the record's LocalidadeID against the
// locality reference map.
func resolveLocalityID(record edne.SourceRecord, localidades map[string]edne.SourceRecord) (string, *UnresolvedReference) {
	id := record.LocalidadeID
	if id == "" {
		return "", &UnresolvedReference{
			PostalCode:    record.CEP,
			AddressType:   record.Class.AddressType(),
			RecordClass:   record.Class.String(),
			ReferenceType: "locality",
			ReferenceID:   id,
			SourceFile:    record.FileName,
		}
	}
	if _, found := localidades[id]; !found {
		return "", &UnresolvedReference{
			PostalCode:    record.CEP,
			AddressType:   record.Class.AddressType(),
			RecordClass:   record.Class.String(),
			ReferenceType: "locality",
			ReferenceID:   id,
			SourceFile:    record.FileName,
		}
	}
	return id, nil
}

// resolveNeighborhoodID resolves the record's BairroID against the
// neighborhood reference map.
func resolveNeighborhoodID(record edne.SourceRecord, bairros map[string]edne.SourceRecord) (string, *UnresolvedReference) {
	id := record.BairroID
	if id == "" {
		return "", nil
	}
	if _, found := bairros[id]; !found {
		return "", &UnresolvedReference{
			PostalCode:    record.CEP,
			AddressType:   record.Class.AddressType(),
			RecordClass:   record.Class.String(),
			ReferenceType: "neighborhood",
			ReferenceID:   id,
			SourceFile:    record.FileName,
		}
	}
	return id, nil
}

// resolveAddress determines the canonical street and additional_information fields.
func resolveAddress(record edne.SourceRecord) (street, additionalInfo string) {
	street = record.Logradouro
	additionalInfo = record.Complemento

	switch record.Class {
	case edne.ClassLogradouro:
		if record.LogradouroTipo != "" {
			street = record.LogradouroTipo + " " + street
		}
		additionalInfo = strings.TrimSpace(strings.TrimPrefix(additionalInfo, "-"))
	case edne.ClassGrandeUsuario, edne.ClassUnidadeOperacional, edne.ClassCPC:
		if record.Nome != "" {
			street = record.Nome
		}
		if record.Endereco != "" {
			additionalInfo = record.Endereco
		}
	}
	return
}

// resolvePostalUnit returns the postal unit name for POSTAL_UNIT records.
func resolvePostalUnit(record edne.SourceRecord, addressType string) string {
	if addressType == "POSTAL_UNIT" {
		return record.Nome
	}
	return ""
}

// buildReferenceRecords extracts distinct locality and neighborhood records
// from the parsed results for seeding the localities and neighborhoods tables.
func buildReferenceRecords(results []fileResult) (localities []edne.SourceRecord, neighborhoods []edne.SourceRecord) {
	seenLocality := make(map[string]bool)
	seenNeighborhood := make(map[string]bool)

	for _, result := range results {
		switch result.layout.Class {
		case edne.ClassLocalidade:
			for _, record := range result.records {
				if record.LocalidadeID != "" && !seenLocality[record.LocalidadeID] {
					seenLocality[record.LocalidadeID] = true
					localities = append(localities, record)
				}
			}
		case edne.ClassBairro:
			for _, record := range result.records {
				if record.BairroID != "" && !seenNeighborhood[record.BairroID] {
					seenNeighborhood[record.BairroID] = true
					neighborhoods = append(neighborhoods, record)
				}
			}
		}
	}
	return
}

package graph

import (
	"errors"

	deque "github.com/MarcinZarkowski/EconGraph/graph/deque"
)

func (g *Graph) impactOf(event *Event) (*EventImpact, error) {

	var res *EventImpact
	var err error
	switch event.EventKind {
	case CompanyEventKind:
		res, err = g.handleCompanyEvent(event)
	case CommodityEventKind:
		res, err = g.handleCommodityEvent(event)
	case SectorEventKind:
		res, err = g.handleSectorEvent(event)
	default:
		return nil, errors.New("unsupported event kind")
	}

	return res, err
}

func IfElse[T any](cond bool, thenDo, elseDo T) T {
	if cond {
		return thenDo
	}
	return elseDo
}

// Events will be described as
// Country or 'GLOBAL' plus commodity
// or
// Company
type EventKind int8

const (
	CommodityEventKind EventKind = iota
	SectorEventKind
	CompanyEventKind
)

type ShockKind int8

const (
	SupplyShockKind ShockKind = iota
	DemandShockKind
)

type Event struct {
	EventKind EventKind
	ShockKind ShockKind
	// Shock intensity (0.0 to 1.0). Multiply by implied volatility normalized to [1.0, 2.0] at event detection to simulate market reaction.
	Magnitude   float32
	GlobalEvent bool

	Commodities []*Commodity
	Countries   []*Country
	Sectors     []*Sector
	Companies   []*Company
}

type EventImpact struct {
	// impacted companies at different breadth
	Companies map[uint32]*CompanyImpactRecord
	Sectors   map[string]*SectorImpactRecord
	Countries map[string]*CountryImpactRecord
}

func NewEventImpact() *EventImpact {
	return &EventImpact{
		Companies: make(map[uint32]*CompanyImpactRecord),
		Sectors:   make(map[string]*SectorImpactRecord),
		Countries: make(map[string]*CountryImpactRecord),
	}
}

type ImpactRecord struct {
	TotalImpact float32
	HitCount    int
	Shocks      []*IncomingShock
}
type CompanyImpactRecord struct {
	Company *Company
	ImpactRecord
}

type SectorImpactRecord struct {
	Sector *Sector
	ImpactRecord
}

type CountryImpactRecord struct {
	Country *Country
	ImpactRecord
}

func NewImpactRecord(shock *IncomingShock) ImpactRecord {
	return ImpactRecord{
		TotalImpact: shock.Impact,
		HitCount:    1,
		Shocks:      []*IncomingShock{shock},
	}
}

func (r *ImpactRecord) AddShock(shock *IncomingShock) {
	r.HitCount++
	r.Shocks = append(r.Shocks, shock)
	r.TotalImpact = proportionalExcessReduction(r.TotalImpact, shock.Impact)
}

type IncomingShock struct {
	FromCompany *Company
	ToCompany   *Company

	Impact float32
	Depth  uint8
}

func proportionalExcessReduction(a, b float32) float32 {
	return 1 - (1-a)*(1-b)
}

func enqueSupplyChainEdges(q *deque.Deque[*IncomingShock], eventKind ShockKind, edges []*SupplyChainEdge, shockDepth uint8, incomingShock float32) {
	// No cycle detection because we want to detect and accumulate impact the more a company is visited,
	// Graph traversal is capped by g.MaxTraversalDepth
	for _, edge := range edges {
		FromCompany := IfElse(eventKind == SupplyShockKind, edge.FromCompany, edge.ToCompany)
		ToCompany := IfElse(eventKind == SupplyShockKind, edge.ToCompany, edge.FromCompany)
		Impact := IfElse(eventKind == SupplyShockKind, edge.SuppliedDependencyWeight, edge.SupplierRevenueWeight)

		element := &IncomingShock{
			FromCompany: FromCompany,
			ToCompany:   ToCompany,
			Impact:      incomingShock * Impact,
			Depth:       shockDepth,
		}
		q.PushBack(element)
	}
}

func recordCompanyImpact(
	company *Company,
	incomingShock *IncomingShock,
	companiesImpacted map[uint32]*CompanyImpactRecord,
) {
	if _, ok := companiesImpacted[company.ID]; !ok {
		companiesImpacted[company.ID] = &CompanyImpactRecord{
			Company:      company,
			ImpactRecord: NewImpactRecord(incomingShock),
		}
	} else {
		companiesImpacted[company.ID].AddShock(incomingShock)
	}
}

func recordMacroImpact(
	edge *MacroRevenueEdge,
	incomingShock *IncomingShock,
	countriesImpacted map[string]*CountryImpactRecord,
	sectorsImpacted map[string]*SectorImpactRecord,
) {

	countryShock := &IncomingShock{
		FromCompany: incomingShock.FromCompany,
		ToCompany:   incomingShock.ToCompany,
		Impact:      incomingShock.Impact * edge.CountryMarketShare,
		Depth:       incomingShock.Depth,
	}

	if _, ok := countriesImpacted[edge.Country.ISO]; !ok {
		countriesImpacted[edge.Country.ISO] = &CountryImpactRecord{
			Country:      edge.Country,
			ImpactRecord: NewImpactRecord(countryShock),
		}
	} else {
		countriesImpacted[edge.Country.ISO].AddShock(countryShock)
	}

	sectorShock := &IncomingShock{
		FromCompany: incomingShock.FromCompany,
		ToCompany:   incomingShock.ToCompany,
		Impact:      incomingShock.Impact * edge.SectorMarketShare,
		Depth:       incomingShock.Depth,
	}

	if _, ok := sectorsImpacted[edge.Sector.Code]; !ok {
		sectorsImpacted[edge.Sector.Code] = &SectorImpactRecord{
			Sector:       edge.Sector,
			ImpactRecord: NewImpactRecord(sectorShock),
		}
	} else {
		sectorsImpacted[edge.Sector.Code].AddShock(sectorShock)
	}

}

func (g *Graph) runTraversal(q *deque.Deque[*IncomingShock], shockKind ShockKind, impact *EventImpact) {

	node, ok := q.PeekFront()
	for ok && node.Depth <= g.MaxTraversalDepth {
		currShock, _ := q.PopFront()

		// accumulate impact on companies, countries and sectors
		recordCompanyImpact(currShock.ToCompany, currShock, impact.Companies)

		for _, edge := range currShock.ToCompany.MacroRevenueEdges {
			recordMacroImpact(edge, currShock, impact.Countries, impact.Sectors)
		}

		// enque edges
		if currShock.Depth+1 <= g.MaxTraversalDepth {
			traverseEdges := IfElse(shockKind == SupplyShockKind, currShock.ToCompany.SuppliesTo, currShock.ToCompany.SuppliedFrom)
			enqueSupplyChainEdges(q, shockKind, traverseEdges, currShock.Depth+1, currShock.Impact)
		}

		node, ok = q.PeekFront()
	}

}

func (g *Graph) handleCompanyEvent(event *Event) (*EventImpact, error) {
	affectedCountries := make(map[string]bool, len(event.Countries))
	for _, c := range event.Countries {
		affectedCountries[c.ISO] = true
	}
	impact := NewEventImpact()

	initialCap := max(len(event.Companies)*512, 256)
	q := deque.NewDeque[*IncomingShock](initialCap)
	for _, company := range event.Companies {
		rootShock := &IncomingShock{
			FromCompany: nil,
			ToCompany:   company,
			Impact:      event.Magnitude,
			Depth:       0,
		}

		// record initial shock to companies, countries, sectors at epicenter of event
		recordCompanyImpact(company, rootShock, impact.Companies)

		for _, edge := range company.MacroRevenueEdges {
			recordMacroImpact(edge, rootShock, impact.Countries, impact.Sectors)
		}

		traverseEdges := IfElse(event.ShockKind == SupplyShockKind, company.SuppliesTo, company.SuppliedFrom)
		for _, edge := range traverseEdges {
			countryKey := IfElse(event.ShockKind == SupplyShockKind, edge.FromCountry.ISO, edge.ToCompany.Country.ISO)
			if !event.GlobalEvent && !affectedCountries[countryKey] {
				continue
			}

			ToCompany := IfElse(event.ShockKind == SupplyShockKind, edge.ToCompany, edge.FromCompany)
			Impact := IfElse(event.ShockKind == SupplyShockKind, edge.SuppliedDependencyWeight, edge.SupplierRevenueWeight)
			element := &IncomingShock{
				FromCompany: company,
				ToCompany:   ToCompany,
				Impact:      event.Magnitude * Impact,
				Depth:       1,
			}

			q.PushBack(element)
		}
	}

	g.runTraversal(q, event.ShockKind, impact)

	return impact, nil
}

func (g *Graph) handleCommodityEvent(event *Event) (*EventImpact, error) {
	if event.ShockKind != SupplyShockKind {
		return nil, errors.New("Commodity events can only ever have shock on supply.")
	}

	affectedCountries := make(map[string]bool, len(event.Countries))
	for _, c := range event.Countries {
		affectedCountries[c.ISO] = true
	}
	impact := NewEventImpact()
	initialCap := max(len(event.Commodities)*512, 256)
	q := deque.NewDeque[*IncomingShock](initialCap)

	for _, commodity := range event.Commodities {
		for _, edge := range commodity.SuppliesTo {
			// only process edges that are impacted by this commodity from the events country list
			if !event.GlobalEvent && !affectedCountries[edge.FromCountry.ISO] {
				continue
			}

			// initialize epicenter impact on company, country, and sector
			rootShock := &IncomingShock{
				FromCompany: nil,
				ToCompany:   edge.ToCompany,
				Impact:      event.Magnitude * edge.SuppliedDependencyWeight,
				Depth:       0,
			}

			recordCompanyImpact(edge.ToCompany, rootShock, impact.Companies)

			for _, revEdge := range edge.ToCompany.MacroRevenueEdges {
				recordMacroImpact(revEdge, rootShock, impact.Countries, impact.Sectors)
			}

			enqueSupplyChainEdges(q, SupplyShockKind, edge.ToCompany.SuppliesTo, 1, rootShock.Impact)

		}
	}

	g.runTraversal(q, SupplyShockKind, impact)
	return impact, nil
}

func (g *Graph) handleSectorEvent(event *Event) (*EventImpact, error) {
	affectedCountries := make(map[string]bool, len(event.Countries))
	for _, c := range event.Countries {
		affectedCountries[c.ISO] = true
	}
	impact := NewEventImpact()
	initialCap := max(len(event.Sectors)*512, 256)
	q := deque.NewDeque[*IncomingShock](initialCap)

	processEpicenter := func(company *Company, weight float32, countryISO string) {
		if !event.GlobalEvent && !affectedCountries[countryISO] {
			return
		}
		rootShock := &IncomingShock{
			FromCompany: nil,
			ToCompany:   company,
			Impact:      event.Magnitude * weight,
			Depth:       0,
		}

		recordCompanyImpact(company, rootShock, impact.Companies)

		for _, revEdge := range company.MacroRevenueEdges {
			recordMacroImpact(revEdge, rootShock, impact.Countries, impact.Sectors)
		}
		nextEdges := IfElse(event.ShockKind == SupplyShockKind, company.SuppliesTo, company.SuppliedFrom)
		enqueSupplyChainEdges(q, event.ShockKind, nextEdges, 1, rootShock.Impact)
	}

	for _, sector := range event.Sectors {
		switch event.ShockKind {
		case SupplyShockKind:
			for _, edge := range sector.SuppliesTo {
				processEpicenter(edge.ToCompany, edge.SuppliedDependencyWeight, edge.FromCountry.ISO)
			}
		case DemandShockKind:
			for _, edge := range sector.MacroRevenueEdges {
				processEpicenter(edge.Company, edge.CompanyRevenueWeight, edge.Country.ISO)
			}
		}
	}

	g.runTraversal(q, event.ShockKind, impact)
	return impact, nil
}

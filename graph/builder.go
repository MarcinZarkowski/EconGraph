package graph

import (
	"context"
	"fmt"
	"time"

	s3Client "github.com/MarcinZarkowski/EconGraph/graph/s3"
	persistedEconGraph "github.com/MarcinZarkowski/EconGraphSchema/clients/go/v1"
)

func NewGraph(DampenerFactor float32, MaxTraversalDepth uint8) (*Graph, error) {

	g := &Graph{
		DampenerFactor:    DampenerFactor,
		MaxTraversalDepth: MaxTraversalDepth,
		CompaniesById:     make(map[uint32]*Company),
		CommoditiesById:   make(map[string]*Commodity),
		SectorByCode:      make(map[SectorKey]*Sector),
		CountriesByIso:    make(map[string]*Country),
	}

	if err := g.buildGraph(); err != nil {
		return nil, err
	}

	return g, nil
}

func (g *Graph) buildGraph() error {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	s3, err := s3Client.NewClient(ctx)
	if err != nil {
		return err
	}

	ctx, cancel = context.WithTimeout(ctx, time.Minute*10)
	defer cancel()

	if err := g.loadCompanies(ctx, s3); err != nil {
		return fmt.Errorf("load companies: %w", err)
	}
	if err := g.loadSectors(ctx, s3); err != nil {
		return fmt.Errorf("load sectors: %w", err)
	}
	if err := g.loadCountries(ctx, s3); err != nil {
		return fmt.Errorf("load countries: %w", err)
	}
	if err := g.loadCommodities(ctx, s3); err != nil {
		return fmt.Errorf("load commodities: %w", err)
	}

	if err := g.loadSupplyChainEdges(ctx, s3); err != nil {
		return fmt.Errorf("load supply chain edges: %w", err)
	}
	if err := g.loadMacroRevenueEdges(ctx, s3); err != nil {
		return fmt.Errorf("load macro revenue edges: %w", err)
	}

	return nil
}

func (g *Graph) loadCompanies(ctx context.Context, s3 *s3Client.Client) error {

	for message, err := range s3Client.NewProtoFileReader[persistedEconGraph.CompanyNode](s3, "companies.parquet").Read(ctx) {
		if err != nil {
			return err
		}

		comp := &Company{
			ID:     message.GetId(),
			CIK:    message.GetCik(),
			Ticker: message.GetTicker(),
			Name:   message.GetName(),
		}

		g.CompaniesById[comp.ID] = comp
	}
	return nil
}

func toSectorTaxonomyKind(taxonomy persistedEconGraph.SectorTaxonomy) (SectorTaxonomyKind, error) {
	switch taxonomy {
	case persistedEconGraph.SectorTaxonomy_BEA:
		return BEATaxonomyKind, nil
	case persistedEconGraph.SectorTaxonomy_ICIO:
		return ICIOTaxonomyKind, nil
	default:
		return 0, fmt.Errorf("invalid sector taxonomy: %v", taxonomy)
	}
}

func (g *Graph) loadSectors(ctx context.Context, s3 *s3Client.Client) error {
	for message, err := range s3Client.NewProtoFileReader[persistedEconGraph.SectorNode](s3, "sectors.parquet").Read(ctx) {
		if err != nil {
			return err
		}

		taxonomy, err := toSectorTaxonomyKind(message.GetTaxonomy())
		if err != nil {
			return err
		}

		sector := &Sector{
			Code:     message.GetCode(),
			Name:     message.GetName(),
			Taxonomy: taxonomy,
		}

		key := SectorKey{Taxonomy: taxonomy, Code: sector.Code}
		g.SectorByCode[key] = sector
	}
	return nil
}

func (g *Graph) loadCountries(ctx context.Context, s3 *s3Client.Client) error {
	for message, err := range s3Client.NewProtoFileReader[persistedEconGraph.CountryNode](s3, "countries.parquet").Read(ctx) {
		if err != nil {
			return err
		}

		country := &Country{
			ISO:  message.GetIso(),
			Name: message.GetName(),
		}

		g.CountriesByIso[country.ISO] = country
	}
	return nil
}

func (g *Graph) loadCommodities(ctx context.Context, s3 *s3Client.Client) error {
	for message, err := range s3Client.NewProtoFileReader[persistedEconGraph.CommodityNode](s3, "commodities.parquet").Read(ctx) {
		if err != nil {
			return err
		}

		beaSector, ok := g.SectorByCode[SectorKey{Taxonomy: BEATaxonomyKind, Code: message.GetBeaSectorCode()}]
		if !ok {
			return fmt.Errorf("BEA sector %s not found", message.GetBeaSectorCode())
		}

		icioSector, ok := g.SectorByCode[SectorKey{Taxonomy: ICIOTaxonomyKind, Code: message.GetIcioSectorCode()}]
		if !ok {
			return fmt.Errorf("ICIO sector %s not found", message.GetIcioSectorCode())
		}

		commodity := &Commodity{
			CommodityCode: message.GetCommodityCode(),
			Name:          message.GetName(),
			BEASector:     beaSector,
			ICIOSector:    icioSector,
		}

		g.CommoditiesById[commodity.CommodityCode] = commodity
	}
	return nil
}

func (g *Graph) loadSupplyChainEdges(ctx context.Context, s3 *s3Client.Client) error {

	for message, err := range s3Client.NewProtoFileReader[persistedEconGraph.SupplyChainEdge](s3, "supply-chain-edges.parquet").Read(ctx) {
		if err != nil {
			return err
		}

		fromCountry, ok := g.CountriesByIso[message.GetFromCountryIso()]
		if !ok {
			return fmt.Errorf("country %s not found, 'FromCountry' should always be present in SupplyChainEdge", message.GetFromCountryIso())
		}

		toCompany, ok := g.CompaniesById[message.GetToCompanyId()]
		if !ok {
			return fmt.Errorf("company %d not found, 'ToCompany' should always be present in SupplyChainEdge", message.GetToCompanyId())
		}

		var supplyKind SupplyKind
		switch message.GetSupplyKind() {
		case persistedEconGraph.SupplyKind_COMMODITY:
			supplyKind = CommoditySupplyKind
		case persistedEconGraph.SupplyKind_COMPANY:
			supplyKind = CompanySupplyKind
		case persistedEconGraph.SupplyKind_SECTOR:
			supplyKind = SectorSupplyKind
		default:
			return fmt.Errorf("edge for company %d has invalid supply kind: %v", message.GetToCompanyId(), message.GetSupplyKind())
		}

		var (
			fromCompany *Company
			fromSector  *Sector
			commodity   *Commodity
		)

		switch supplyKind {
		case CommoditySupplyKind:
			commodity, ok = g.CommoditiesById[message.GetCommodityCode()]
			if !ok {
				return fmt.Errorf("commodity %s not found, required for COMMODITY supply edge", message.GetCommodityCode())
			}
		case CompanySupplyKind:
			fromCompany, ok = g.CompaniesById[message.GetFromCompanyId()]
			if !ok {
				return fmt.Errorf("supplier company %d not found, required for COMPANY supply edge", message.GetFromCompanyId())
			}
		case SectorSupplyKind:
			tax, err := toSectorTaxonomyKind(message.GetFromSectorTaxonomy())
			if err != nil {
				return fmt.Errorf("sector supply taxonomy: %w", err)
			}
			fromSector, ok = g.SectorByCode[SectorKey{Taxonomy: tax, Code: message.GetFromSectorCode()}]
			if !ok {
				return fmt.Errorf("supplier sector %s not found, required for SECTOR supply edge", message.GetFromSectorCode())
			}
		}

		supplyChainEdge := &SupplyChainEdge{
			SupplierRevenueWeight:    message.GetSupplierRevenueWeight(),
			SuppliedDependencyWeight: message.GetSuppliedDependencyWeight(),

			ToCompany:   toCompany,
			FromCompany: fromCompany,
			Commodity:   commodity,
			FromCountry: fromCountry,
			FromSector:  fromSector,
			SupplyKind:  supplyKind,
		}

		// hook it up to graph
		toCompany.SuppliedFrom = append(toCompany.SuppliedFrom, supplyChainEdge)

		switch supplyKind {
		case CommoditySupplyKind:
			commodity.SuppliesTo = append(commodity.SuppliesTo, supplyChainEdge)
		case CompanySupplyKind:
			fromCompany.SuppliesTo = append(fromCompany.SuppliesTo, supplyChainEdge)
		case SectorSupplyKind:
			fromSector.SuppliesTo = append(fromSector.SuppliesTo, supplyChainEdge)
		}

	}
	return nil
}

func (g *Graph) loadMacroRevenueEdges(ctx context.Context, s3 *s3Client.Client) error {

	for message, err := range s3Client.NewProtoFileReader[persistedEconGraph.MacroRevenueEdge](s3, "macro-revenue-edges.parquet").Read(ctx) {
		if err != nil {
			return err
		}

		SectorTaxonomy, err := toSectorTaxonomyKind(message.GetSectorTaxonomy())
		if err != nil {
			return err
		}

		Sector, SectorFound := g.SectorByCode[SectorKey{Taxonomy: SectorTaxonomy, Code: message.GetSectorCode()}]
		if !SectorFound {
			return fmt.Errorf("sector %s not found, 'Sector' should allways be present in MacroRevenueEdge", message.GetSectorCode())
		}

		Country, CountryFound := g.CountriesByIso[message.GetCountryIso()]
		if !CountryFound {
			return fmt.Errorf("country %s not found, 'Country' should allways be present in MacroRevenueEdge", message.GetCountryIso())
		}

		Company, CompanyFound := g.CompaniesById[message.GetCompanyId()]
		if !CompanyFound {
			return fmt.Errorf("company %d not found, 'Company' should allways be present in MacroRevenueEdge", message.GetCompanyId())
		}

		var currency [3]byte
		copy(currency[:], message.GetCurrency())

		macroRevenueEdge := &MacroRevenueEdge{
			Sector:  Sector,
			Country: Country,
			Company: Company,

			SectorMarketShare:    message.GetSectorMarketShare(),
			CountryMarketShare:   message.GetCountryMarketShare(),
			CompanyRevenueWeight: message.GetCompanyRevenueWeight(),

			Revenue: message.GetRevenue(),
			Scale:   message.GetScale(),

			Currency: currency,
		}

		Company.MacroRevenueEdges = append(Company.MacroRevenueEdges, macroRevenueEdge)
		Sector.MacroRevenueEdges = append(Sector.MacroRevenueEdges, macroRevenueEdge)
		Country.MacroRevenueEdges = append(Country.MacroRevenueEdges, macroRevenueEdge)
	}

	return nil
}

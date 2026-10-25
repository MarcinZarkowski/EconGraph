package graph

// NodeKind identifies what type of node this is in graph traversals
type NodeKind uint8

const (
	CompanyNodeKind NodeKind = iota
	CommodityNodeKind
	SectorNodeKind
	CountryNodeKind
)

type SectorKey struct {
	Taxonomy SectorTaxonomyKind
	Code     string
}

type Graph struct {
	DampenerFactor    float32 // shock is lost with every hop
	MaxTraversalDepth uint8   // breadth search from the event depth

	// There are limited ways to query the impact of an event:
	// look at enum EventType

	CompaniesById   map[uint32]*Company
	CommoditiesById map[string]*Commodity
	SectorByCode    map[SectorKey]*Sector
	CountriesByIso  map[string]*Country

	// Ones that affect a commodity in a country or are 'GLOBAL'
	// Commodities can only supply to other companies
	//CommoditySupplies map[string]map[string][]*SupplyChainEdge

	// Ones that affect a sector in a country or are 'GLOBAL'
	// Sectors can supply to companies and also provide revenue
	// SectorSupplies map[string]map[string][]*SupplyChainEdge
	// SectorDemand   map[string]map[string][]*MacroRevenueEdge

	// Ones that affect a company in a country or are 'GLOBAL'
	// Companies can supply to other companies and also provide revenue
	// CompanySupplies map[string]map[string][]*SupplyChainEdge
	// CompanyDemand   map[string]map[string][]*MacroRevenueEdge
}

// MacroRevenueEdge: Company -> Sector (in a destination Country)
type MacroRevenueEdge struct {
	Sector  *Sector  `json:"-"`
	Country *Country `json:"-"`
	Company *Company `json:"-"`

	Revenue float64 `json:"revenue"` // Dollar amount ($)
	Scale   int64   `json:"scale"`   // e.g. 1000000

	// Macro/Systemic: % of the destination Sector's total output represented by this company
	SectorMarketShare float32 `json:"sector_market_share,omitempty"`

	// Macro/Systemic: % of the destination Country's total GDP/output represented by this company
	CountryMarketShare float32 `json:"country_market_share,omitempty"`

	// Micro/Company: % of this company's total revenue derived from this sector/country
	CompanyRevenueWeight float32 `json:"company_weight"`

	Currency [3]byte `json:"currency"` // 'USD'
}

type SupplyKind uint8

const (
	CommoditySupplyKind SupplyKind = iota
	CompanySupplyKind
	SectorSupplyKind
)

// SupplyChainEdge: Commodity OR Supplier Company OR Sector -> Company
type SupplyChainEdge struct {
	// Upstream: % of Supplier's total revenue derived from ToCompany (0.0 to 1.0)
	SupplierRevenueWeight float32 `json:"supplier_revenue_weight,omitempty"`

	// Downstream: How critically ToCompany depends on this supplier (0.0 to 1.0)
	SuppliedDependencyWeight float32 `json:"dependency_weight"`

	ToCompany *Company `json:"to_company"`

	// Country should always be supplied or 'GLOBAL' when there isn't specific country
	FromCountry *Country `json:"-"`

	// 1. If supplied by a Commodity:
	Commodity *Commodity `json:"-"`

	// 2. If supplied by a Company:
	FromCompany *Company `json:"-"`

	// 3. If supplied by Sector
	FromSector *Sector `json:"-"`

	SupplyKind SupplyKind
}

// Company Node
type Company struct {
	CIK     string   `json:"cik,omitempty"`
	Ticker  string   `json:"ticker,omitempty"`
	Name    string   `json:"name,omitempty"`
	Country *Country `json:"country,omitempty"`

	// Manager type is used in Financial Economy mapping
	//IsManager *Manager
	// Upstream dependencies (who supplies this company)
	SuppliedFrom []*SupplyChainEdge `json:"supply_sources"`

	// What companies does this company supplies to
	// all of these SupplyChainEdges should have SupplyKind CompanySupplyKind
	SuppliesTo []*SupplyChainEdge `json:"supplies_to"`

	// Downstream revenue (what sectors/countries buy from this company)
	MacroRevenueEdges []*MacroRevenueEdge `json:"revenue_destinations"`

	ID uint32
}

// Commodity Node (e.g. 'USGS_LITHIUM', 'EIA_EPCWTI')
type Commodity struct {
	CommodityCode string `json:"commodity_code"` // 'USGS_LITHIUM'
	Name          string `json:"name"`           // 'Lithium Carbonate & Hydroxide'
	BEASector     *Sector
	ICIOSector    *Sector

	SuppliesTo []*SupplyChainEdge
}

// Country Node
type Country struct {
	ISO  string `json:"iso"`  // 'US', 'TW', 'CL'
	Name string `json:"name"` // 'United States', 'Taiwan'

	MacroRevenueEdges []*MacroRevenueEdge
}

type SectorTaxonomyKind uint8

const (
	ICIOTaxonomyKind SectorTaxonomyKind = iota
	BEATaxonomyKind
)

// Sector Node (e.g. ICIO or BEA industry sector)
type Sector struct {
	Code string `json:"code"` // 'C26'
	Name string `json:"name"` // 'Computer, electronic and optical products'

	SuppliesTo        []*SupplyChainEdge
	MacroRevenueEdges []*MacroRevenueEdge
	Taxonomy          SectorTaxonomyKind
}

// SecurityClass distinguishes the exact type of 13F holding
// type SecurityClass uint8

// const (
// 	EquityClass SecurityClass = iota
// 	CallOptionClass
// 	PutOptionClass
// 	ConvertibleDebtClass // Disclosed as "PRN" (Principal) instead of "SH" (Shares)
// )

// type Manager struct {
// 	ID   uint64 // Internal graph index
// 	CIK  string
// 	Name string

// 	// If this manager is ALSO a publicly traded company in your supply chain
// 	// graph (e.g., BlackRock, Goldman Sachs), link to its Company node here.
// 	// If it is a private, this remains nil.
// 	IsCompany *Company

// 	InvestsIn []*InvestmentEdge
// }

// // InvestmentEdge represents the directional graph relationship: Manager -> Issuer
// type InvestmentEdge struct {
// 	ByManager *Manager // e.g., the Filer's CIK (Central Index Key)
// 	InCompany *Company // e.g., the Issuer's CUSIP

// 	// Security Identification
// 	CUSIP        string
// 	TitleOfClass string // e.g., "COM", "CLASS A", "SPON ADR"
// 	Class        SecurityClass

// 	// Financials
// 	ValueUSD float64 // SEC XML <value>
// 	Shares   float64 // SEC XML <sshPrnamt>. Float handles PRN debt amounts better.

// 	// 13F Specific Metadata
// 	Discretion string // "SOLE", "DFND" (Defined), or "OTR" (Other)

// 	// Voting Authority Breakdown (must sum to total Shares)
// 	VotingSole   int64
// 	VotingShared int64
// 	VotingNone   int64
// }

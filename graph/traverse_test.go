package graph

import (
	"math"
	"testing"
)

// Helper to check float equality within tolerance
func almostEqual(a, b float32, epsilon float32) bool {
	return math.Abs(float64(a-b)) <= float64(epsilon)
}

// 1. Multi-Path and Cycles with Reverberation
func TestHandleCompanyEvent_MultiPathAndCycles(t *testing.T) {
	countryUS := &Country{ISO: "US", Name: "United States"}
	compA := &Company{ID: 1, Name: "CompanyA", Country: countryUS}
	compB := &Company{ID: 2, Name: "CompanyB", Country: countryUS}
	compC := &Company{ID: 3, Name: "CompanyC", Country: countryUS}

	// A -> B (A supplies B)
	edgeAB := &SupplyChainEdge{
		FromCompany:              compA,
		ToCompany:                compB,
		FromCountry:              countryUS,
		SuppliedDependencyWeight: 0.5,
	}
	compA.SuppliesTo = append(compA.SuppliesTo, edgeAB)
	compB.SuppliedFrom = append(compB.SuppliedFrom, edgeAB)

	// A -> C (A supplies C directly)
	edgeAC := &SupplyChainEdge{
		FromCompany:              compA,
		ToCompany:                compC,
		FromCountry:              countryUS,
		SuppliedDependencyWeight: 0.3,
	}
	compA.SuppliesTo = append(compA.SuppliesTo, edgeAC)
	compC.SuppliedFrom = append(compC.SuppliedFrom, edgeAC)

	// B -> C (B supplies C)
	edgeBC := &SupplyChainEdge{
		FromCompany:              compB,
		ToCompany:                compC,
		FromCountry:              countryUS,
		SuppliedDependencyWeight: 0.4,
	}
	compB.SuppliesTo = append(compB.SuppliesTo, edgeBC)
	compC.SuppliedFrom = append(compC.SuppliedFrom, edgeBC)

	// Cycle: C -> B (C supplies B)
	edgeCB := &SupplyChainEdge{
		FromCompany:              compC,
		ToCompany:                compB,
		FromCountry:              countryUS,
		SuppliedDependencyWeight: 0.9,
	}
	compC.SuppliesTo = append(compC.SuppliesTo, edgeCB)
	compB.SuppliedFrom = append(compB.SuppliedFrom, edgeCB)

	g := &Graph{
		DampenerFactor:    0.8,
		MaxTraversalDepth: 3,
	}

	event := &Event{
		EventKind:   CompanyEventKind,
		ShockKind:   SupplyShockKind,
		Magnitude:   1.0,
		GlobalEvent: true,
		Companies:   []*Company{compA},
	}

	impact, err := g.handleCompanyEvent(event)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if impact == nil {
		t.Fatal("Expected non-nil EventImpact")
	}

	// Verify Root Node
	recA := impact.Companies[compA.ID]
	if recA == nil || recA.TotalImpact != 1.0 {
		t.Fatalf("Expected Root Company A to have TotalImpact 1.0, got %v", recA)
	}

	// Verify B received shock
	recB := impact.Companies[compB.ID]
	if recB == nil || recB.TotalImpact <= 0 {
		t.Fatalf("Expected Company B to have impact > 0, got %v", recB)
	}

	// Verify C received shocks from multiple paths and cycle reverberation (A -> C, A -> B -> C, and A -> C -> B -> C)
	recC := impact.Companies[compC.ID]
	if recC == nil {
		t.Fatal("Expected Company C to be impacted")
	}

	if recC.HitCount != 3 {
		t.Fatalf("Expected Company C to have HitCount == 3 (from A, B, and reverberation), got %d", recC.HitCount)
	}

	if len(recC.Shocks) != 3 {
		t.Fatalf("Expected 3 incoming shock records for Company C, got %d", len(recC.Shocks))
	}

	if recC.TotalImpact >= 1.0 || recC.TotalImpact <= 0 {
		t.Fatalf("Expected Company C impact to be bounded between 0 and 1.0, got %f", recC.TotalImpact)
	}
}

// 2. Demand Shock: Upstream propagation via SuppliedFrom and SupplierRevenueWeight
func TestHandleCompanyEvent_DemandShock(t *testing.T) {
	// Supply chain flow: Upstream Tier 2 (Supplier A) -> Tier 1 (Supplier B) -> Downstream Retailer C
	// When Retailer C suffers a demand shock, shock propagates upstream: C -> B -> A
	countryUS := &Country{ISO: "US", Name: "United States"}
	compA := &Company{ID: 10, Name: "ComponentSupplierA", Country: countryUS}
	compB := &Company{ID: 20, Name: "AssemblySupplierB", Country: countryUS}
	compC := &Company{ID: 30, Name: "RetailerC", Country: countryUS}

	// Edge A -> B: Supplier A derives 50% revenue from B
	edgeAB := &SupplyChainEdge{
		FromCompany:           compA,
		ToCompany:             compB,
		FromCountry:           countryUS,
		SupplierRevenueWeight: 0.5,
	}
	compA.SuppliesTo = append(compA.SuppliesTo, edgeAB)
	compB.SuppliedFrom = append(compB.SuppliedFrom, edgeAB)

	// Edge B -> C: Supplier B derives 60% revenue from C
	edgeBC := &SupplyChainEdge{
		FromCompany:           compB,
		ToCompany:             compC,
		FromCountry:           countryUS,
		SupplierRevenueWeight: 0.6,
	}
	compB.SuppliesTo = append(compB.SuppliesTo, edgeBC)
	compC.SuppliedFrom = append(compC.SuppliedFrom, edgeBC)

	g := &Graph{
		MaxTraversalDepth: 3,
	}

	event := &Event{
		EventKind:   CompanyEventKind,
		ShockKind:   DemandShockKind,
		Magnitude:   0.8,
		GlobalEvent: true,
		Companies:   []*Company{compC}, // Demand shock originates at Retailer C
	}

	impact, err := g.handleCompanyEvent(event)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Retailer C (Root): Impact = 0.8
	recC := impact.Companies[compC.ID]
	if recC == nil || !almostEqual(recC.TotalImpact, 0.8, 1e-4) {
		t.Fatalf("Expected Retailer C impact 0.8, got %v", recC)
	}

	// Supplier B (Hop 1 upstream): Impact = 0.8 * 0.6 = 0.48
	recB := impact.Companies[compB.ID]
	if recB == nil || !almostEqual(recB.TotalImpact, 0.48, 1e-4) {
		t.Fatalf("Expected Supplier B impact ~0.48, got %v", recB)
	}

	// Supplier A (Hop 2 upstream): Impact = 0.48 * 0.5 = 0.24
	recA := impact.Companies[compA.ID]
	if recA == nil || !almostEqual(recA.TotalImpact, 0.24, 1e-4) {
		t.Fatalf("Expected Supplier A impact ~0.24, got %v", recA)
	}
}

// 3. Country Filter: First hop filtered by affected country, subsequent hops propagate globally
func TestHandleCompanyEvent_CountryFilter_FirstHopOnly(t *testing.T) {
	countryTW := &Country{ISO: "TW", Name: "Taiwan"}
	countryDE := &Country{ISO: "DE", Name: "Germany"}
	countryUS := &Country{ISO: "US", Name: "United States"}
	countryJP := &Country{ISO: "JP", Name: "Japan"}

	compA := &Company{ID: 100, Name: "GlobalTechA", Country: countryUS}
	compB := &Company{ID: 101, Name: "SubcontractorB", Country: countryUS}
	compC := &Company{ID: 102, Name: "DistributorC", Country: countryDE}
	compD := &Company{ID: 103, Name: "CustomerD", Country: countryJP}

	// A -> B via TW facility
	edgeAB := &SupplyChainEdge{
		FromCompany:              compA,
		ToCompany:                compB,
		FromCountry:              countryTW,
		SuppliedDependencyWeight: 0.8,
	}
	compA.SuppliesTo = append(compA.SuppliesTo, edgeAB)
	compB.SuppliedFrom = append(compB.SuppliedFrom, edgeAB)

	// A -> C via DE facility
	edgeAC := &SupplyChainEdge{
		FromCompany:              compA,
		ToCompany:                compC,
		FromCountry:              countryDE,
		SuppliedDependencyWeight: 0.5,
	}
	compA.SuppliesTo = append(compA.SuppliesTo, edgeAC)
	compC.SuppliedFrom = append(compC.SuppliedFrom, edgeAC)

	// B -> D (Hop 2 from US to JP)
	edgeBD := &SupplyChainEdge{
		FromCompany:              compB,
		ToCompany:                compD,
		FromCountry:              countryUS,
		SuppliedDependencyWeight: 0.5,
	}
	compB.SuppliesTo = append(compB.SuppliesTo, edgeBD)
	compD.SuppliedFrom = append(compD.SuppliedFrom, edgeBD)

	g := &Graph{
		MaxTraversalDepth: 3,
	}

	// Event is localized to TW only
	event := &Event{
		EventKind:   CompanyEventKind,
		ShockKind:   SupplyShockKind,
		Magnitude:   1.0,
		GlobalEvent: false,
		Countries:   []*Country{countryTW},
		Companies:   []*Company{compA},
	}

	impact, err := g.handleCompanyEvent(event)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Root A must be impacted
	if impact.Companies[compA.ID] == nil {
		t.Fatal("Expected Root A to be impacted")
	}

	// B should be impacted (hop 1 matches TW)
	if impact.Companies[compB.ID] == nil {
		t.Fatal("Expected Company B to be impacted because edge A->B originates in TW")
	}

	// C should NOT be impacted (hop 1 is DE, not affected by TW event)
	if impact.Companies[compC.ID] != nil {
		t.Fatalf("Company C should NOT be impacted; edge A->C is DE but event is in TW")
	}

	// D SHOULD be impacted (hop 2 propagates globally beyond initial TW event boundary)
	recD := impact.Companies[compD.ID]
	if recD == nil {
		t.Fatal("Expected Company D (Japan) to be impacted via Hop 2 propagation from B")
	}
	// Expected impact on D = 1.0 (magnitude) * 0.8 (A->B) * 0.5 (B->D) = 0.4
	if !almostEqual(recD.TotalImpact, 0.4, 1e-4) {
		t.Fatalf("Expected Company D impact ~0.4, got %f", recD.TotalImpact)
	}
}

// 4. MaxTraversalDepth boundary enforcement
func TestHandleCompanyEvent_MaxTraversalDepth_Boundaries(t *testing.T) {
	countryUS := &Country{ISO: "US", Name: "United States"}
	comps := make([]*Company, 5)
	for i := range comps {
		comps[i] = &Company{ID: uint32(i + 1), Name: string(rune('A' + i)), Country: countryUS}
	}
	for i := 0; i < len(comps)-1; i++ {
		edge := &SupplyChainEdge{
			FromCompany:              comps[i],
			ToCompany:                comps[i+1],
			FromCountry:              countryUS,
			SuppliedDependencyWeight: 0.5,
		}
		comps[i].SuppliesTo = append(comps[i].SuppliesTo, edge)
		comps[i+1].SuppliedFrom = append(comps[i+1].SuppliedFrom, edge)
	}

	// Case 1: MaxTraversalDepth = 0 (Only Root A impacted)
	g0 := &Graph{MaxTraversalDepth: 0}
	event := &Event{
		EventKind:   CompanyEventKind,
		ShockKind:   SupplyShockKind,
		Magnitude:   1.0,
		GlobalEvent: true,
		Companies:   []*Company{comps[0]},
	}
	impact0, err := g0.handleCompanyEvent(event)
	if err != nil {
		t.Fatalf("Error at depth 0: %v", err)
	}
	if len(impact0.Companies) != 1 || impact0.Companies[comps[0].ID] == nil {
		t.Fatalf("At MaxDepth 0, only Root A should be impacted, got %d companies", len(impact0.Companies))
	}

	// Case 2: MaxTraversalDepth = 2 (Root A + Hop 1 B + Hop 2 C)
	g2 := &Graph{MaxTraversalDepth: 2}
	impact2, err := g2.handleCompanyEvent(event)
	if err != nil {
		t.Fatalf("Error at depth 2: %v", err)
	}
	if len(impact2.Companies) != 3 {
		t.Fatalf("At MaxDepth 2, exactly 3 companies (A, B, C) should be impacted, got %d", len(impact2.Companies))
	}
	if impact2.Companies[comps[3].ID] != nil {
		t.Fatal("Company D (hop 3) should NOT be impacted when MaxTraversalDepth is 2")
	}
}

// 5. Diamond Graph: Multiple independent incoming paths to the same node
func TestHandleCompanyEvent_DiamondGraph_IndependentAccumulation(t *testing.T) {
	// A -> B (0.4) \
	//               -> D
	// A -> C (0.5) /
	countryUS := &Country{ISO: "US", Name: "United States"}
	compA := &Company{ID: 201, Name: "SourceA", Country: countryUS}
	compB := &Company{ID: 202, Name: "BranchB", Country: countryUS}
	compC := &Company{ID: 203, Name: "BranchC", Country: countryUS}
	compD := &Company{ID: 204, Name: "SinkD", Country: countryUS}

	edgeAB := &SupplyChainEdge{FromCompany: compA, ToCompany: compB, FromCountry: countryUS, SuppliedDependencyWeight: 0.4}
	compA.SuppliesTo = append(compA.SuppliesTo, edgeAB)
	compB.SuppliedFrom = append(compB.SuppliedFrom, edgeAB)

	edgeAC := &SupplyChainEdge{FromCompany: compA, ToCompany: compC, FromCountry: countryUS, SuppliedDependencyWeight: 0.5}
	compA.SuppliesTo = append(compA.SuppliesTo, edgeAC)
	compC.SuppliedFrom = append(compC.SuppliedFrom, edgeAC)

	// B supplies D (0.3)
	edgeBD := &SupplyChainEdge{FromCompany: compB, ToCompany: compD, FromCountry: countryUS, SuppliedDependencyWeight: 0.3}
	compB.SuppliesTo = append(compB.SuppliesTo, edgeBD)
	compD.SuppliedFrom = append(compD.SuppliedFrom, edgeBD)

	// C supplies D (0.4)
	edgeCD := &SupplyChainEdge{FromCompany: compC, ToCompany: compD, FromCountry: countryUS, SuppliedDependencyWeight: 0.4}
	compC.SuppliesTo = append(compC.SuppliesTo, edgeCD)
	compD.SuppliedFrom = append(compD.SuppliedFrom, edgeCD)

	g := &Graph{
		MaxTraversalDepth: 2,
	}

	event := &Event{
		EventKind:   CompanyEventKind,
		ShockKind:   SupplyShockKind,
		Magnitude:   1.0,
		GlobalEvent: true,
		Companies:   []*Company{compA},
	}

	impact, err := g.handleCompanyEvent(event)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	recD := impact.Companies[compD.ID]
	if recD == nil {
		t.Fatal("Expected Sink D to be impacted")
	}

	if recD.HitCount != 2 {
		t.Fatalf("Expected Sink D to have HitCount == 2 (from B and C), got %d", recD.HitCount)
	}

	// Path 1 (via B): 1.0 * 0.4 * 0.3 = 0.12
	// Path 2 (via C): 1.0 * 0.5 * 0.4 = 0.20
	// Proportional excess reduction: 1 - (1 - 0.12) * (1 - 0.20) = 1 - 0.88 * 0.80 = 1 - 0.704 = 0.296
	expectedImpact := float32(1.0 - (1.0-0.12)*(1.0-0.20))
	if !almostEqual(recD.TotalImpact, expectedImpact, 1e-4) {
		t.Fatalf("Expected Sink D TotalImpact ~%f, got %f", expectedImpact, recD.TotalImpact)
	}
}

// 6. Multiple Root Companies in a single event
func TestHandleCompanyEvent_MultipleRoots(t *testing.T) {
	countryUS := &Country{ISO: "US", Name: "United States"}
	rootA1 := &Company{ID: 301, Name: "RootA1", Country: countryUS}
	rootA2 := &Company{ID: 302, Name: "RootA2", Country: countryUS}
	commonB := &Company{ID: 303, Name: "CommonSupplierB", Country: countryUS}

	edge1 := &SupplyChainEdge{FromCompany: rootA1, ToCompany: commonB, FromCountry: countryUS, SuppliedDependencyWeight: 0.3}
	rootA1.SuppliesTo = append(rootA1.SuppliesTo, edge1)
	commonB.SuppliedFrom = append(commonB.SuppliedFrom, edge1)

	edge2 := &SupplyChainEdge{FromCompany: rootA2, ToCompany: commonB, FromCountry: countryUS, SuppliedDependencyWeight: 0.5}
	rootA2.SuppliesTo = append(rootA2.SuppliesTo, edge2)
	commonB.SuppliedFrom = append(commonB.SuppliedFrom, edge2)

	g := &Graph{
		MaxTraversalDepth: 2,
	}

	event := &Event{
		EventKind:   CompanyEventKind,
		ShockKind:   SupplyShockKind,
		Magnitude:   0.9,
		GlobalEvent: true,
		Companies:   []*Company{rootA1, rootA2},
	}

	impact, err := g.handleCompanyEvent(event)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Both roots must be in result with magnitude 0.9
	if rec := impact.Companies[rootA1.ID]; rec == nil || !almostEqual(rec.TotalImpact, 0.9, 1e-4) {
		t.Fatalf("Expected RootA1 to have impact 0.9, got %v", rec)
	}
	if rec := impact.Companies[rootA2.ID]; rec == nil || !almostEqual(rec.TotalImpact, 0.9, 1e-4) {
		t.Fatalf("Expected RootA2 to have impact 0.9, got %v", rec)
	}

	// Common node should receive shocks from both roots
	recB := impact.Companies[commonB.ID]
	if recB == nil {
		t.Fatal("Expected CommonSupplierB to be impacted")
	}
	if recB.HitCount != 2 {
		t.Fatalf("Expected CommonSupplierB to have HitCount == 2, got %d", recB.HitCount)
	}

	// Shock 1: 0.9 * 0.3 = 0.27
	// Shock 2: 0.9 * 0.5 = 0.45
	// Combined: 1 - (1 - 0.27)*(1 - 0.45) = 1 - 0.73 * 0.55 = 1 - 0.4015 = 0.5985
	expectedB := float32(1.0 - (1.0-0.27)*(1.0-0.45))
	if !almostEqual(recB.TotalImpact, expectedB, 1e-4) {
		t.Fatalf("Expected CommonSupplierB impact ~%f, got %f", expectedB, recB.TotalImpact)
	}
}

// 7. Edge Cases: Disconnected Node and Zero Magnitude
func TestHandleCompanyEvent_EdgeCases(t *testing.T) {
	countryUS := &Country{ISO: "US", Name: "United States"}
	isolated := &Company{ID: 401, Name: "IsolatedCompany", Country: countryUS}

	g := &Graph{
		MaxTraversalDepth: 5,
	}

	// Case A: Isolated company (no edges)
	eventA := &Event{
		EventKind:   CompanyEventKind,
		ShockKind:   SupplyShockKind,
		Magnitude:   0.75,
		GlobalEvent: true,
		Companies:   []*Company{isolated},
	}
	impactA, err := g.handleCompanyEvent(eventA)
	if err != nil {
		t.Fatalf("Unexpected error for isolated company: %v", err)
	}
	if len(impactA.Companies) != 1 || impactA.Companies[isolated.ID].TotalImpact != 0.75 {
		t.Fatalf("Isolated company should have impact 0.75 and no other nodes, got %v", impactA.Companies)
	}

	// Case B: Zero Magnitude Event
	eventB := &Event{
		EventKind:   CompanyEventKind,
		ShockKind:   SupplyShockKind,
		Magnitude:   0.0,
		GlobalEvent: true,
		Companies:   []*Company{isolated},
	}
	impactB, err := g.handleCompanyEvent(eventB)
	if err != nil {
		t.Fatalf("Unexpected error for zero magnitude: %v", err)
	}
	if impactB.Companies[isolated.ID].TotalImpact != 0.0 {
		t.Fatalf("Expected zero magnitude to result in 0.0 impact, got %f", impactB.Companies[isolated.ID].TotalImpact)
	}
}

// 8. Macro Revenue Edges: Verify Country and Sector impacts are accurately calculated and aggregated
func TestHandleCompanyEvent_MacroRevenueEdges_CountryAndSectorImpact(t *testing.T) {
	countryUS := &Country{ISO: "US", Name: "United States"}
	countryDE := &Country{ISO: "DE", Name: "Germany"}

	sectorTech := &Sector{Taxonomy: ICIOTaxonomyKind, Code: "C26", Name: "Computer and Electronic Products"}
	sectorAuto := &Sector{Taxonomy: ICIOTaxonomyKind, Code: "C29", Name: "Motor Vehicles"}

	compA := &Company{ID: 501, Name: "TechGiantA", Country: countryUS}
	compB := &Company{ID: 502, Name: "Tier1SupplierB", Country: countryUS}

	// A supplies B with dependency 0.5
	edgeAB := &SupplyChainEdge{
		FromCompany:              compA,
		ToCompany:                compB,
		FromCountry:              countryUS,
		SuppliedDependencyWeight: 0.5,
	}
	compA.SuppliesTo = append(compA.SuppliesTo, edgeAB)
	compB.SuppliedFrom = append(compB.SuppliedFrom, edgeAB)

	// Macro Revenue Edges for Comp A:
	// A represents 10% of US GDP and 20% of US Tech sector output
	compA.MacroRevenueEdges = append(compA.MacroRevenueEdges, &MacroRevenueEdge{
		Company:            compA,
		Country:            countryUS,
		Sector:             sectorTech,
		CountryMarketShare: 0.10,
		SectorMarketShare:  0.20,
	})
	// A also represents 5% of German GDP and 15% of German Auto sector
	compA.MacroRevenueEdges = append(compA.MacroRevenueEdges, &MacroRevenueEdge{
		Company:            compA,
		Country:            countryDE,
		Sector:             sectorAuto,
		CountryMarketShare: 0.05,
		SectorMarketShare:  0.15,
	})

	// Macro Revenue Edges for Comp B:
	// B represents 8% of US GDP and 12% of US Tech sector output
	compB.MacroRevenueEdges = append(compB.MacroRevenueEdges, &MacroRevenueEdge{
		Company:            compB,
		Country:            countryUS,
		Sector:             sectorTech,
		CountryMarketShare: 0.08,
		SectorMarketShare:  0.12,
	})

	g := &Graph{
		MaxTraversalDepth: 2,
	}

	event := &Event{
		EventKind:   CompanyEventKind,
		ShockKind:   SupplyShockKind,
		Magnitude:   1.0,
		GlobalEvent: true,
		Companies:   []*Company{compA},
	}

	impact, err := g.handleCompanyEvent(event)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// 1. Verify Companies
	if len(impact.Companies) != 2 {
		t.Fatalf("Expected 2 impacted companies, got %d", len(impact.Companies))
	}
	// Comp A: Root shock = 1.0
	if recA := impact.Companies[compA.ID]; recA == nil || recA.TotalImpact != 1.0 {
		t.Fatalf("Expected Comp A TotalImpact 1.0, got %v", recA)
	}
	// Comp B: Hop 1 shock = 1.0 * 0.5 = 0.5
	if recB := impact.Companies[compB.ID]; recB == nil || !almostEqual(recB.TotalImpact, 0.5, 1e-4) {
		t.Fatalf("Expected Comp B TotalImpact 0.5, got %v", recB)
	}

	// 2. Verify Countries
	// US received shocks from both Comp A (root: 1.0 * 0.10 = 0.10) and Comp B (hop 1: 0.5 * 0.08 = 0.04)
	cUS := impact.Countries["US"]
	if cUS == nil {
		t.Fatal("Expected Country US to be impacted")
	}
	if cUS.HitCount != 2 {
		t.Fatalf("Expected Country US to have 2 hits (from A and B), got %d", cUS.HitCount)
	}
	// Combined US impact: 1 - (1 - 0.10) * (1 - 0.04) = 1 - 0.90 * 0.96 = 1 - 0.864 = 0.136
	expectedUS := float32(1.0 - (1.0-0.10)*(1.0-0.04))
	if !almostEqual(cUS.TotalImpact, expectedUS, 1e-4) {
		t.Fatalf("Expected Country US impact ~%f, got %f", expectedUS, cUS.TotalImpact)
	}

	// Germany received shock only from Comp A: 1.0 * 0.05 = 0.05
	cDE := impact.Countries["DE"]
	if cDE == nil {
		t.Fatal("Expected Country DE to be impacted")
	}
	if cDE.HitCount != 1 || !almostEqual(cDE.TotalImpact, 0.05, 1e-4) {
		t.Fatalf("Expected Country DE impact 0.05 with 1 hit, got %v", cDE)
	}

	// 3. Verify Sectors
	// Sector Tech (C26) received shocks from both Comp A (1.0 * 0.20 = 0.20) and Comp B (0.5 * 0.12 = 0.06)
	sTech := impact.Sectors["C26"]
	if sTech == nil {
		t.Fatal("Expected Sector Tech (C26) to be impacted")
	}
	if sTech.HitCount != 2 {
		t.Fatalf("Expected Sector C26 to have 2 hits, got %d", sTech.HitCount)
	}
	// Combined Tech impact: 1 - (1 - 0.20) * (1 - 0.06) = 1 - 0.80 * 0.94 = 1 - 0.752 = 0.248
	expectedTech := float32(1.0 - (1.0-0.20)*(1.0-0.06))
	if !almostEqual(sTech.TotalImpact, expectedTech, 1e-4) {
		t.Fatalf("Expected Sector Tech impact ~%f, got %f", expectedTech, sTech.TotalImpact)
	}

	// Sector Auto (C29) received shock only from Comp A: 1.0 * 0.15 = 0.15
	sAuto := impact.Sectors["C29"]
	if sAuto == nil {
		t.Fatal("Expected Sector Auto (C29) to be impacted")
	}
	if sAuto.HitCount != 1 || !almostEqual(sAuto.TotalImpact, 0.15, 1e-4) {
		t.Fatalf("Expected Sector Auto impact 0.15 with 1 hit, got %v", sAuto)
	}
}

// 9. Commodity Event: Shock to raw material propagating downstream to companies and macro sectors/countries
func TestHandleCommodityEvent(t *testing.T) {
	sectorMining := &Sector{Taxonomy: ICIOTaxonomyKind, Code: "B07", Name: "Mining of Metal Ores"}
	lithium := &Commodity{CommodityCode: "LITHIUM", Name: "Lithium", ICIOSector: sectorMining}
	countryCL := &Country{ISO: "CL", Name: "Chile"}
	countryUS := &Country{ISO: "US", Name: "United States"}
	sectorAuto := &Sector{Taxonomy: ICIOTaxonomyKind, Code: "C29", Name: "Motor Vehicles"}

	batteryMaker := &Company{ID: 601, Name: "BatteryMaker", Country: countryUS}
	evMaker := &Company{ID: 602, Name: "EVMaker", Country: countryUS}

	// Lithium -> BatteryMaker (Commodity supply edge from Chile)
	edgeRes := &SupplyChainEdge{
		Commodity:                lithium,
		ToCompany:                batteryMaker,
		FromCountry:              countryCL,
		SuppliedDependencyWeight: 0.8,
	}
	lithium.SuppliesTo = append(lithium.SuppliesTo, edgeRes)
	batteryMaker.SuppliedFrom = append(batteryMaker.SuppliedFrom, edgeRes)

	// BatteryMaker -> EVMaker (Company supply edge)
	edgeComp := &SupplyChainEdge{
		FromCompany:              batteryMaker,
		ToCompany:                evMaker,
		FromCountry:              countryUS,
		SuppliedDependencyWeight: 0.5,
	}
	batteryMaker.SuppliesTo = append(batteryMaker.SuppliesTo, edgeComp)
	evMaker.SuppliedFrom = append(evMaker.SuppliedFrom, edgeComp)

	// EVMaker has MacroRevenueEdge to US Motor Vehicles (C29)
	evMaker.MacroRevenueEdges = append(evMaker.MacroRevenueEdges, &MacroRevenueEdge{
		Company:            evMaker,
		Country:            countryUS,
		Sector:             sectorAuto,
		CountryMarketShare: 0.05,
		SectorMarketShare:  0.25,
	})

	g := &Graph{
		MaxTraversalDepth: 2,
	}

	// Shock on Chilean Lithium of magnitude 0.9
	event := &Event{
		EventKind:   CommodityEventKind,
		ShockKind:   SupplyShockKind,
		Magnitude:   0.9,
		GlobalEvent: false,
		Countries:   []*Country{countryCL},
		Commodities: []*Commodity{lithium},
	}

	impact, err := g.handleCommodityEvent(event)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// 1. Verify BatteryMaker (Direct consumer of Lithium, Depth 0)
	// Expected impact = 0.9 * 0.8 = 0.72
	recBattery := impact.Companies[batteryMaker.ID]
	if recBattery == nil || !almostEqual(recBattery.TotalImpact, 0.72, 1e-4) {
		t.Fatalf("Expected BatteryMaker impact ~0.72, got %v", recBattery)
	}

	// 2. Verify EVMaker (Downstream tier 1, Depth 1)
	// Expected impact = 0.72 * 0.5 = 0.36
	recEV := impact.Companies[evMaker.ID]
	if recEV == nil || !almostEqual(recEV.TotalImpact, 0.36, 1e-4) {
		t.Fatalf("Expected EVMaker impact ~0.36, got %v", recEV)
	}

	// 3. Verify Macro Sector and Country
	// Sector C29: EVMaker impact 0.36 * 0.25 SectorMarketShare = 0.09
	sAuto := impact.Sectors["C29"]
	if sAuto == nil || !almostEqual(sAuto.TotalImpact, 0.09, 1e-4) {
		t.Fatalf("Expected Sector C29 impact ~0.09, got %v", sAuto)
	}

	// Country US: EVMaker impact 0.36 * 0.05 CountryMarketShare = 0.018
	cUS := impact.Countries["US"]
	if cUS == nil || !almostEqual(cUS.TotalImpact, 0.018, 1e-4) {
		t.Fatalf("Expected Country US impact ~0.018, got %v", cUS)
	}

	// Error case: Commodity event with DemandShockKind must return an error
	badEvent := &Event{
		EventKind:   CommodityEventKind,
		ShockKind:   DemandShockKind,
		Commodities: []*Commodity{lithium},
	}
	if _, err := g.handleCommodityEvent(badEvent); err == nil {
		t.Fatal("Expected error when running handleCommodityEvent with DemandShockKind")
	}
}

// 10. Sector Event: Supply Shock (Sector -> Company -> Company)
func TestHandleSectorEvent_SupplyShock(t *testing.T) {
	countryTW := &Country{ISO: "TW", Name: "Taiwan"}
	countryUS := &Country{ISO: "US", Name: "United States"}
	sectorFoundry := &Sector{Taxonomy: ICIOTaxonomyKind, Code: "C26", Name: "Semiconductors"}

	compA := &Company{ID: 701, Name: "ChipDesignerA", Country: countryUS}
	compB := &Company{ID: 702, Name: "DeviceMakerB", Country: countryUS}

	// Sector (Taiwan Foundry C26) supplies ChipDesignerA
	edgeSec := &SupplyChainEdge{
		FromSector:               sectorFoundry,
		ToCompany:                compA,
		FromCountry:              countryTW,
		SuppliedDependencyWeight: 0.6,
	}
	sectorFoundry.SuppliesTo = append(sectorFoundry.SuppliesTo, edgeSec)
	compA.SuppliedFrom = append(compA.SuppliedFrom, edgeSec)

	// ChipDesignerA supplies DeviceMakerB
	edgeAB := &SupplyChainEdge{
		FromCompany:              compA,
		ToCompany:                compB,
		FromCountry:              countryUS,
		SuppliedDependencyWeight: 0.5,
	}
	compA.SuppliesTo = append(compA.SuppliesTo, edgeAB)
	compB.SuppliedFrom = append(compB.SuppliedFrom, edgeAB)

	g := &Graph{
		MaxTraversalDepth: 2,
	}

	event := &Event{
		EventKind:   SectorEventKind,
		ShockKind:   SupplyShockKind,
		Magnitude:   1.0,
		GlobalEvent: false,
		Countries:   []*Country{countryTW}, // Localized to Taiwan
		Sectors:     []*Sector{sectorFoundry},
	}

	impact, err := g.handleSectorEvent(event)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Comp A: Root shock = 1.0 * 0.6 = 0.6
	recA := impact.Companies[compA.ID]
	if recA == nil || !almostEqual(recA.TotalImpact, 0.6, 1e-4) {
		t.Fatalf("Expected Comp A impact ~0.6, got %v", recA)
	}

	// Comp B: Hop 1 shock = 0.6 * 0.5 = 0.3
	recB := impact.Companies[compB.ID]
	if recB == nil || !almostEqual(recB.TotalImpact, 0.3, 1e-4) {
		t.Fatalf("Expected Comp B impact ~0.3, got %v", recB)
	}
}

// 11. Sector Event: Demand Shock (Sector demand drops, propagating upstream via MacroRevenueEdge to Company.SuppliedFrom)
func TestHandleSectorEvent_DemandShock(t *testing.T) {
	countryUS := &Country{ISO: "US", Name: "United States"}
	sectorAuto := &Sector{Taxonomy: ICIOTaxonomyKind, Code: "C29", Name: "Motor Vehicles"}

	autoOEM := &Company{ID: 801, Name: "AutoOEM", Country: countryUS}
	partsSupplier := &Company{ID: 802, Name: "PartsSupplier", Country: countryUS}

	// AutoOEM derives 80% of revenue from US Motor Vehicles Sector (C29)
	autoOEM.MacroRevenueEdges = append(autoOEM.MacroRevenueEdges, &MacroRevenueEdge{
		Company:              autoOEM,
		Country:              countryUS,
		Sector:               sectorAuto,
		CompanyRevenueWeight: 0.8,
		CountryMarketShare:   0.05,
		SectorMarketShare:    0.20,
	})
	sectorAuto.MacroRevenueEdges = append(sectorAuto.MacroRevenueEdges, autoOEM.MacroRevenueEdges[0])

	// PartsSupplier supplies AutoOEM (PartsSupplier derives 50% revenue from AutoOEM)
	edgeSupply := &SupplyChainEdge{
		FromCompany:           partsSupplier,
		ToCompany:             autoOEM,
		FromCountry:           countryUS,
		SupplierRevenueWeight: 0.5,
	}
	partsSupplier.SuppliesTo = append(partsSupplier.SuppliesTo, edgeSupply)
	autoOEM.SuppliedFrom = append(autoOEM.SuppliedFrom, edgeSupply)

	g := &Graph{
		MaxTraversalDepth: 2,
	}

	// Global demand shock of 0.5 on Auto Sector
	event := &Event{
		EventKind:   SectorEventKind,
		ShockKind:   DemandShockKind,
		Magnitude:   0.5,
		GlobalEvent: true,
		Sectors:     []*Sector{sectorAuto},
	}

	impact, err := g.handleSectorEvent(event)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// AutoOEM: Direct hit = 0.5 * 0.8 = 0.4
	recOEM := impact.Companies[autoOEM.ID]
	if recOEM == nil || !almostEqual(recOEM.TotalImpact, 0.4, 1e-4) {
		t.Fatalf("Expected AutoOEM impact ~0.4, got %v", recOEM)
	}

	// PartsSupplier: Cascaded upstream hit = 0.4 * 0.5 = 0.2
	recParts := impact.Companies[partsSupplier.ID]
	if recParts == nil || !almostEqual(recParts.TotalImpact, 0.2, 1e-4) {
		t.Fatalf("Expected PartsSupplier impact ~0.2, got %v", recParts)
	}

	// Macro Sector C29: AutoOEM impact 0.4 * 0.20 SectorMarketShare = 0.08
	sAuto := impact.Sectors["C29"]
	if sAuto == nil || !almostEqual(sAuto.TotalImpact, 0.08, 1e-4) {
		t.Fatalf("Expected Sector C29 impact ~0.08, got %v", sAuto)
	}
}

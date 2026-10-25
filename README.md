# EconGraph

A graph-based economic modeling system that simulates shock propagation through global supply chains and revenue networks.

## Graph Structure

The graph contains **four node types** connected by weighted edges:

| Node Type | Description | Key Attributes |
|-----------|-------------|----------------|
| Company | Publicly traded entities (via CIK/ticker) | Suppliers, customers, revenue by sector/country |
| Commodity | Raw materials (USGS, EIA sources) | Linked to BEA & ICIO sectors |
| Sector | Industry classifications (BEA & ICIO taxonomies) | Supply chain & revenue relationships |
| Country | Nations (ISO codes) | Macro revenue aggregation |

## Flow Chart Diagram

```
SUPPLY CHAIN (upstream -> downstream)
======================================

  [Commodity] + [Country]          [Sector] + [Country]          [Company] + [Country]
       |                                   |                              |
       | supplies                          | supplies                     | supplies
       v                                   v                              v
    +---------------------------------------------------------------+
    |                        [Company]                              |
    |  (receives inputs, produces outputs, generates revenue)       |
    +---------------------------------------------------------------+
       |                                   |                              |
       | supplies                          | supplies                     | supplies
       v                                   v                              v
  [Company] + [Country]             [Company] + [Country]          [Company] + [Country]


REVENUE FLOWS (company -> sector/country)
=========================================

    +---------------------------------------------------------------+
    |                        [Company]                              |
    +---------------------------------------------------------------+
       |                                   |                              |
       | revenue                           | revenue                      | revenue
       v                                   v                              v
  [Sector] + [Country]              [Sector] + [Country]           [Sector] + [Country]
```

## Edge Types

**SupplyChainEdge** — Upstream to Downstream dependencies
- Commodity + Country -> Company
- Sector + Country -> Company  
- Company + Country -> Company
- Weights: `SupplierRevenueWeight` (upstream), `SuppliedDependencyWeight` (downstream)

**MacroRevenueEdge** — Company to Sector/Country revenue flows
- Company -> Sector + Country
- Weights: `SectorMarketShare`, `CountryMarketShare`, `CompanyRevenueWeight`

## Shock Propagation

The engine models three event types with **breadth-first traversal** up to `MaxTraversalDepth`:

| Event Kind | Shock Types | Propagation Direction |
|------------|-------------|----------------------|
| Company | Supply / Demand | Upstream (supply) or Downstream (demand) |
| Commodity | Supply only | Downstream to consuming companies |
| Sector | Supply / Demand | Both directions via supply chain & revenue edges |

**Dampening**: Each hop multiplies impact by `DampenerFactor` (< 1.0). Multiple paths to the same node accumulate impact via `proportionalExcessReduction`.

## Data Source

Graph data loads from **Parquet files on S3** (via `EconGraphSchema` protobuf definitions):
- `companies.parquet`, `sectors.parquet`, `countries.parquet`, `commodities.parquet`
- `supply-chain-edges.parquet`, `macro-revenue-edges.parquet`

## Usage

```go
g, err := graph.NewGraph(0.85, 3) // dampener=0.85, maxDepth=3
impact, err := g.ImpactOf(&graph.Event{
    EventKind: graph.CompanyEventKind,
    ShockKind: graph.SupplyShockKind,
    Magnitude: 0.5,
    Companies: []*graph.Company{targetCompany},
})
```
package model

// Import catalogs stay server-side; the dashboard remains request-focused.
type ImportCatalog struct {
	Known            bool
	Movies           []ImportMovie
	Series           []ImportSeries
	HistoryKnown     bool
	HistoryAttempted bool
	History          []ImportEvent
}

type ImportMovie struct {
	ID, TMDBID int
	HasFile    *bool
}
type ImportSeries struct{ ID, TVDBID int }
type ImportEvent struct {
	ID                           int64
	MovieID, SeriesID, EpisodeID int
	Type, Date                   string
}

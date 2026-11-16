package upstream

import "reqarr/model"

func MovieImportCatalog(movies map[int]Movie) model.ImportCatalog {
	catalog := model.ImportCatalog{Known: true}
	for _, movie := range movies {
		catalog.Movies = append(catalog.Movies, model.ImportMovie{ID: movie.ID, TMDBID: movie.TMDBID, HasFile: movie.HasFile})
	}
	return catalog
}

func SeriesImportCatalog(series map[int]Series) model.ImportCatalog {
	catalog := model.ImportCatalog{Known: true}
	for _, show := range series {
		catalog.Series = append(catalog.Series, model.ImportSeries{ID: show.ID, TVDBID: show.TVDBID})
	}
	return catalog
}

func ImportHistory(records []HistoryRecord) []model.ImportEvent {
	events := make([]model.ImportEvent, 0, len(records))
	for _, record := range records {
		events = append(events, model.ImportEvent{ID: record.ID, MovieID: record.MovieID, SeriesID: record.SeriesID, EpisodeID: record.EpisodeID, Type: record.EventType, Date: record.Date})
	}
	return events
}

package dataimport

// ProgressEvent wird vom OnProgress-Callback der Engine an den Aufrufer gemeldet.
type ProgressEvent struct {
	Phase            int
	PhaseName        string
	PhasesTotal      int
	Table            string
	EntitiesTotal    int
	EntitiesImported int
	EntitiesSkipped  int
	EntitiesFailed   int
	FilesTotal       int
	FilesImported    int
	BytesImported    int64
	Message          string
}

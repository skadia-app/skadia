package handler

const (
	// IP literals keep handler tests independent from external DNS while still
	// exercising the same public-shaped URL validation path.
	handlerTestRasterURLTemplate        = "https://203.0.113.10/{z}/{x}/{y}.png"
	handlerTestOtherRasterURLTemplate   = "https://203.0.113.11/{z}/{x}/{y}.png"
	handlerTestUpdatedRasterURLTemplate = "https://203.0.113.12/{z}/{x}/{y}.png"
	handlerTestVectorStyleURL           = "https://203.0.113.13/style.json"
)

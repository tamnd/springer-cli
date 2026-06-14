package springer

// Paper is an academic paper from Springer Nature via CrossRef.
type Paper struct {
	Rank      int    `json:"rank"`
	DOI       string `json:"doi"`
	Title     string `json:"title"`
	Authors   string `json:"authors"`
	Journal   string `json:"journal"`
	Published string `json:"published"`
	Cited     int    `json:"cited"`
	URL       string `json:"url"`
}

// wire types for CrossRef API

type wireResponse struct {
	Message wireMessage `json:"message"`
}

type wireMessage struct {
	TotalResults int        `json:"total-results"`
	NextCursor   string     `json:"next-cursor"`
	Items        []wireItem `json:"items"`
}

type wireItem struct {
	DOI            string       `json:"DOI"`
	Title          []string     `json:"title"`
	Author         []wireAuthor `json:"author"`
	Published      wireDate     `json:"published"`
	Type           string       `json:"type"`
	ContainerTitle []string     `json:"container-title"`
	IsReferencedBy int          `json:"is-referenced-by-count"`
}

type wireAuthor struct {
	Given  string `json:"given"`
	Family string `json:"family"`
}

type wireDate struct {
	DateParts [][]int `json:"date-parts"`
}

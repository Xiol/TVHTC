package main

type TVHJob struct {
	Path        string `json:"path"`
	Filename    string `json:"fname"`
	Channel     string `json:"channel"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Description string `json:"description"`
	DBID        int64  `json:"-"`
}

package mybookmarks

import "time"

type Bookmark struct {
	ID        int       `json:"id"`
	URL       string    `sql:"type:text" json:"url"`
	Title     string    `sql:"type:text" json:"title"`
	Note      string    `sql:"type:text" json:"note"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type BookmarkWithTags struct {
	Bookmark
	Tags string `json:"tags"`
}

type Tag struct {
	ID        int `json:"id"`
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type BookmarkTag struct {
	ID           int `json:"id"`
	BookmarkID   int
	TagID        int
	DisplayOrder int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

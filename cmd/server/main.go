package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"

	_ "github.com/go-sql-driver/mysql"
	"github.com/hnakamur/mybookmarks"
	"github.com/jinzhu/gorm"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/zenazn/goji"
	"github.com/zenazn/goji/web"
)

var sqlDriverName = os.Getenv("SQL_DRIVER_NAME")
var db gorm.DB

func openDB() (gorm.DB, error) {
	return gorm.Open(sqlDriverName, os.Getenv("SQL_DATA_SOURCE"))
}

func renderStatus(w http.ResponseWriter, status string) {
	w.Write([]byte(fmt.Sprintf(`{"status":"%s"}`, status)))
}

func apiGridBookmarks(c web.C, w http.ResponseWriter, r *http.Request) {
	db, err := openDB()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		renderStatus(w, "error")
		return
	}
	defer db.Close()

	command := r.FormValue("cmd")
	switch command {
	case "get-records":
		limit, err := strconv.Atoi(r.FormValue("limit"))
		if err != nil {
			renderStatus(w, "error")
			return
		}
		offset, err := strconv.Atoi(r.FormValue("offset"))
		if err != nil {
			renderStatus(w, "error")
			return
		}
		var count int
		db.Debug().Table("bookmarks").Count(&count)
		bookmarks := []mybookmarks.BookmarkWithTags{}
		var joins string
		switch sqlDriverName {
		case "mysql", "sqlite3":
			joins = `left join (select bookmark_tags.bookmark_id, group_concat(tags.name, ' ') as tags
				from bookmark_tags join tags on (bookmark_tags.tag_id = tags.id)
				group by bookmark_tags.bookmark_id
				order by bookmark_tags.display_order) t
				on (bookmarks.id = t.bookmark_id)`
		case "postgres":
			joins = `left join (
					select bookmark_id, string_agg(name, ' ') as tags
					from (
						select bookmark_tags.bookmark_id, tags.name
						from bookmark_tags join tags on (bookmark_tags.tag_id = tags.id)
						order by bookmark_tags.bookmark_id, bookmark_tags.display_order
					) as bt
					group by bookmark_id
				) t on (bookmarks.id = t.bookmark_id)`
		}
		db.Debug().Table("bookmarks").Select("bookmarks.id, bookmarks.title, bookmarks.url, bookmarks.note, bookmarks.created_at, bookmarks.updated_at, t.tags").Joins(joins).Order(
			"bookmarks.updated_at desc").Offset(offset).Limit(limit).Find(&bookmarks)
		v := map[string]interface{}{
			"total":   count,
			"records": bookmarks,
		}
		encoder := json.NewEncoder(w)
		encoder.Encode(v)
	case "save-records":
		sepRe := regexp.MustCompile("[, ]+")
		for i := 0; ; i++ {
			value, ok := getPostFormFirstValue(r, fmt.Sprintf("changes[%d][recid]", i))
			if !ok {
				break
			}
			recid, err := strconv.Atoi(value)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			bookmark := mybookmarks.Bookmark{}
			db.First(&bookmark, recid)
			dirty := false
			if value, ok := getPostFormFirstValue(r, fmt.Sprintf("changes[%d][title]", i)); ok {
				if bookmark.Title != value {
					bookmark.Title = value
					dirty = true
				}
			}
			if value, ok := getPostFormFirstValue(r, fmt.Sprintf("changes[%d][url]", i)); ok {
				if bookmark.URL != value {
					bookmark.URL = value
					dirty = true
				}
			}
			if value, ok := getPostFormFirstValue(r, fmt.Sprintf("changes[%d][note]", i)); ok {
				if bookmark.Note != value {
					bookmark.Note = value
					dirty = true
				}
			}
			if dirty {
				db.Debug().Save(&bookmark)
			}
			if value, ok := getPostFormFirstValue(r, fmt.Sprintf("changes[%d][tags]", i)); ok {
				log.Printf("i=%d, tags=%s", i, value)
				names := []string{}
				if value != "" {
					for _, name := range sepRe.Split(value, -1) {
						if name != "" {
							names = append(names, name)
						}
					}
				}

				tags := make([]mybookmarks.Tag, len(names))
				if len(names) > 0 {
					foundTags := []mybookmarks.Tag{}
					db.Debug().Where("name in (?)", names).Find(&foundTags)

					for i, name := range names {
						tag, ok := findTagByName(foundTags, name)
						if !ok {
							tag = mybookmarks.Tag{Name: name}
							db.Debug().Save(&tag)
							log.Printf("After save tag: %v", tag)
						}
						tags[i] = tag
					}
				}

				deleteTagIDs := []int{}
				foundBookmarkTags := []mybookmarks.BookmarkTag{}
				db.Debug().Where("bookmark_id = ?", bookmark.ID).Find(&foundBookmarkTags)
				for _, bookmarkTag := range foundBookmarkTags {
					if _, ok := findTagByID(tags, bookmarkTag.TagID); !ok {
						deleteTagIDs = append(deleteTagIDs, bookmarkTag.TagID)
					}
				}
				if len(deleteTagIDs) > 0 {
					db.Debug().Where("bookmark_id = ? and tag_id in (?)", bookmark.ID, deleteTagIDs).Delete(mybookmarks.BookmarkTag{})
					db.Debug().Where("id in (?) and not exists (select null from bookmark_tags where bookmark_tags.id = tags.id)", deleteTagIDs).Delete(mybookmarks.Tag{})
				}

				for i, tag := range tags {
					bookmarkTag, ok := findBookmarkTagByTagID(foundBookmarkTags, tag.ID)
					if ok {
						if bookmarkTag.DisplayOrder != i {
							bookmarkTag.DisplayOrder = i
							db.Debug().Save(&bookmarkTag)
						}
					} else {
						bookmarkTag := mybookmarks.BookmarkTag{
							BookmarkID:   bookmark.ID,
							TagID:        tag.ID,
							DisplayOrder: i,
						}
						db.Debug().Save(&bookmarkTag)
					}
				}
			}
		}

		if db.Error != nil {
			log.Printf("failed to save. err=%s", db.Error)
			renderStatus(w, "error")
			return
		}
		renderStatus(w, "success")
	case "delete-records":
		err := r.ParseForm()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			renderStatus(w, "error")
			return
		}
		values, ok := r.PostForm["selected[]"]
		if ok {
			bookmarkIDs := []int{}
			for _, value := range values {
				bookmarkID, err := strconv.Atoi(value)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				bookmarkIDs = append(bookmarkIDs, bookmarkID)
			}
			db.Debug().Where("id in (?)", bookmarkIDs).Delete(mybookmarks.Bookmark{})
			db.Debug().Where("id in (select distinct tag_id from bookmark_tags where bookmark_id in (?)) and not exists (select null from bookmark_tags where tag_id = tags.id and bookmark_id not in (?))", bookmarkIDs, bookmarkIDs).Delete(mybookmarks.Tag{})
			db.Debug().Where("bookmark_id in (?)", bookmarkIDs).Delete(mybookmarks.BookmarkTag{})
			if db.Error != nil {
				log.Printf("failed to save. err=%s", db.Error)
				renderStatus(w, "error")
				return
			}
		}
		renderStatus(w, "success")
	}
}

func findTagByName(tags []mybookmarks.Tag, name string) (tag mybookmarks.Tag, found bool) {
	for _, tag := range tags {
		if tag.Name == name {
			return tag, true
		}
	}
	return mybookmarks.Tag{}, false
}

func findTagByID(tags []mybookmarks.Tag, id int) (tag mybookmarks.Tag, found bool) {
	for _, tag := range tags {
		if tag.ID == id {
			return tag, true
		}
	}
	return mybookmarks.Tag{}, false
}

func findBookmarkTagByTagID(bookmarkTags []mybookmarks.BookmarkTag, tagID int) (bookmarkTag mybookmarks.BookmarkTag, found bool) {
	for _, bookmarkTag := range bookmarkTags {
		if bookmarkTag.TagID == tagID {
			return bookmarkTag, true
		}
	}
	return mybookmarks.BookmarkTag{}, false
}

func getPostFormFirstValue(r *http.Request, name string) (string, bool) {
	values, ok := r.PostForm[name]
	if ok {
		return values[0], true
	} else {
		return "", false
	}
}

func main() {
	goji.Post("/api/grid/bookmarks", apiGridBookmarks)
	goji.Get("/*", http.FileServer(http.Dir("assets")))
	goji.Serve()
}

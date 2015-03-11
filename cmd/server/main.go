package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"

	"github.com/hnakamur/mybookmarks"
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/zenazn/goji"
	"github.com/zenazn/goji/web"
)

const driverName = "sqlite3"

var db gorm.DB

func openDB() (gorm.DB, error) {
	return gorm.Open(driverName, "gorm.db")
}

func init() {
	log.Print("init start")
	db, err := openDB()
	if err != nil {
		log.Fatalf("failed to open database. %s", err)
	}
	defer db.Close()

	db.AutoMigrate(&mybookmarks.Bookmark{}, &mybookmarks.Tag{}, &mybookmarks.BookmarkTag{})
	db.Model(&mybookmarks.Tag{}).AddUniqueIndex("idx_tag_name", "name")
	db.Model(&mybookmarks.BookmarkTag{}).AddUniqueIndex("idx_bookmark_tag_bookmark_id_tag_id", "bookmark_id", "tag_id")

	db.Delete(mybookmarks.Bookmark{})

	bookmark := mybookmarks.Bookmark{Title: "Go web site", URL: "http://golang.org"}
	db.Create(&bookmark)

	bookmark = mybookmarks.Bookmark{Title: "Goji API document", URL: "http://godoc.org/github.com/zenazn/goji"}
	db.Create(&bookmark)

	bookmark = mybookmarks.Bookmark{Title: "Gorm API document", URL: "http://godoc.org/github.com/jinzhu/gorm"}
	db.Create(&bookmark)

	log.Print("init exit")
}

func renderStatus(w http.ResponseWriter, status string) {
	w.Write([]byte(fmt.Sprintf(`{"status":"%s"}`, status)))
}

func apiGridBookmarks(c web.C, w http.ResponseWriter, r *http.Request) {
	command := r.FormValue("cmd")
	log.Printf("command=%s", command)
	db, err := openDB()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		renderStatus(w, "error")
		return
	}
	defer db.Close()

	switch command {
	case "get-records":
		bookmarks := []mybookmarks.Bookmark{}
		db.Order("updated_at desc").Find(&bookmarks)
		v := map[string]interface{}{
			"total":   len(bookmarks),
			"records": bookmarks,
		}
		encoder := json.NewEncoder(w)
		encoder.Encode(v)
	case "save-records":
		err := r.ParseForm()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			renderStatus(w, "error")
			return
		}
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
				if value != "" {
					names := []string{}
					for _, name := range sepRe.Split(value, -1) {
						if name != "" {
							names = append(names, name)
						}
					}

					foundTags := []mybookmarks.Tag{}
					db.Debug().Where("name in (?)", names).Find(&foundTags)

					tags := make([]mybookmarks.Tag, len(names))
					for i, name := range names {
						tag, ok := findTagByName(foundTags, name)
						if !ok {
							tag = mybookmarks.Tag{Name: name}
							db.Debug().Save(&tag)
							log.Printf("After save tag: %v", tag)
						}
						tags[i] = tag
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

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/hnakamur/mybookmarks"
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/zenazn/goji"
	"github.com/zenazn/goji/web"
)

var db gorm.DB

func openDB() (gorm.DB, error) {
	return gorm.Open("sqlite3", "gorm.db")
}

func init() {
	log.Print("init start")
	db, err := openDB()
	if err != nil {
		log.Fatalf("failed to open database. %s", err)
	}
	defer db.Close()

	db.DropTableIfExists(&mybookmarks.Bookmark{})
	db.CreateTable(&mybookmarks.Bookmark{})

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
			if value, ok := getPostFormFirstValue(r, fmt.Sprintf("changes[%d][title]", i)); ok {
				bookmark.Title = value
			}
			if value, ok := getPostFormFirstValue(r, fmt.Sprintf("changes[%d][url]", i)); ok {
				bookmark.URL = value
			}
			db.Debug().Save(&bookmark)
		}

		if db.Error != nil {
			log.Printf("failed to save. err=%s", db.Error)
			renderStatus(w, "error")
			return
		}
		renderStatus(w, "success")
	}
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

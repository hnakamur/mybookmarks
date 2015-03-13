package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/hnakamur/mybookmarks"
	"github.com/jinzhu/gorm"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

var sqlDriverName = os.Getenv("SQL_DRIVER_NAME")
var db gorm.DB

func readBookmarks(reader io.Reader) ([]mybookmarks.BookmarkWithTags, error) {
	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		return nil, err
	}
	bookmarks := []mybookmarks.BookmarkWithTags{}
	bookmark := mybookmarks.BookmarkWithTags{}
	pending := false
	doc.Find("dt a,dd").Each(func(_ int, s *goquery.Selection) {
		if err != nil {
			return
		}
		switch s.Nodes[0].Data {
		case "a":
			if pending {
				bookmarks = append(bookmarks, bookmark)
				bookmark = mybookmarks.BookmarkWithTags{}
			}

			bookmark.Title = s.Text()
			href, ok := s.Attr("href")
			if !ok {
				err = fmt.Errorf("no href in %s", s.Text())
				return
			}
			bookmark.URL = href

			tags, ok := s.Attr("tags")
			if !ok {
				err = fmt.Errorf("no href in %s", s.Text())
				return
			}
			bookmark.Tags = tags

			addDateStr, ok := s.Attr("add_date")
			if !ok {
				err = fmt.Errorf("no add_date in %s", s.Text())
				return
			}
			var addDateInt int64
			addDateInt, err = strconv.ParseInt(addDateStr, 10, 0)
			if err != nil {
				return
			}
			bookmark.CreatedAt = time.Unix(addDateInt, 0)
			bookmark.UpdatedAt = bookmark.CreatedAt

			pending = true
		case "dd":
			bookmark.Note = s.Text()
			pending = false
		}
	})
	if err != nil {
		return nil, err
	}
	if pending {
		bookmarks = append(bookmarks, bookmark)
	}

	return bookmarks, nil
}

func openDB() (gorm.DB, error) {
	return gorm.Open(sqlDriverName, os.Getenv("SQL_DATA_SOURCE"))
}

func saveBookmarks(bookmarks []mybookmarks.BookmarkWithTags) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	gorm.DefaultCallback.Create().Remove("gorm:update_time_stamp_when_create")
	for i := len(bookmarks) - 1; i >= 0; i-- {
		bookmark := bookmarks[i]
		db.Save(&bookmark.Bookmark)

		tagTexts := strings.Split(bookmark.Tags, ",")
		for i, tagText := range tagTexts {
			tag := mybookmarks.Tag{}
			if db.Where("name = ?", tagText).First(&tag).Error == gorm.RecordNotFound {
				tag.Name = tagText
				tag.CreatedAt = bookmark.CreatedAt
				tag.UpdatedAt = bookmark.UpdatedAt
				db.Save(&tag)
			}
			bookmarkTag := mybookmarks.BookmarkTag{
				BookmarkID:   bookmark.ID,
				TagID:        tag.ID,
				DisplayOrder: i,
				CreatedAt:    bookmark.CreatedAt,
				UpdatedAt:    bookmark.UpdatedAt,
			}
			db.Save(&bookmarkTag)
		}
	}

	return db.Error
}

func main() {
	if len(os.Args) != 2 {
		fmt.Printf("Usage: delicious2csv bookmarks.html\n")
		os.Exit(1)
	}

	reader, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}
	defer reader.Close()
	bookmarks, err := readBookmarks(reader)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

	err = saveBookmarks(bookmarks)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}
}

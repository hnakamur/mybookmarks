package main

import (
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/hnakamur/mybookmarks"
	"github.com/jinzhu/gorm"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

var sqlDriverName = os.Getenv("SQL_DRIVER_NAME")
var db gorm.DB

func openDB() (gorm.DB, error) {
	return gorm.Open(sqlDriverName, os.Getenv("SQL_DATA_SOURCE"))
}

func main() {
	db, err := openDB()
	if err != nil {
		log.Fatalf("failed to open database. %s", err)
	}
	defer db.Close()

	db.AutoMigrate(&mybookmarks.Bookmark{}, &mybookmarks.Tag{}, &mybookmarks.BookmarkTag{})
	db.Model(&mybookmarks.Tag{}).AddUniqueIndex("idx_tag_name", "name")
	db.Model(&mybookmarks.BookmarkTag{}).AddUniqueIndex("idx_bookmark_tag_bookmark_id_tag_id", "bookmark_id", "tag_id")
}

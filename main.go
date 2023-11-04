package main

import (
	"fmt"
	"log"
	"os"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
)

func getPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = ":3000"
	} else {
		port = ":" + port
	}

	return port
}

func main() {
	// Connect to the PostgreSQL database using the data source name
	fmt.Println(os.Getenv("DATABASE_PRIVATE_URL"))
	fmt.Println(os.Getenv("test"))
	
	db, err := sqlx.Connect("postgres", os.Getenv("DATABASE_URL"))
    if err != nil {
		log.Fatalln(err)
	}

	// // Ping the database to ensure it's reachable
	if err := db.Ping(); err != nil {
		log.Fatalln("Failed to ping the database:", err)
	} else {
		log.Println("Successfully connected to the database.")
	}

	defer db.Close()

	engine := html.New("./views", ".html")
	app := fiber.New(fiber.Config{Views: engine})

	app.Get("/", func(c *fiber.Ctx) error {
		return c.Render("index",  fiber.Map{
            "Title": "Hello, World!",
        })
	})

	app.Listen(getPort())
}

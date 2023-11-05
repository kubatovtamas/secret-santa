package main

import (
	// "fmt"
	"log"
	"os"
    "time"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/csrf"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/template/html/v2"
	"golang.org/x/crypto/bcrypt"
)

var schema = `
TRUNCATE TABLE room CASCADE;
TRUNCATE TABLE participant CASCADE;

DROP TABLE IF EXISTS participant;
DROP TABLE IF EXISTS room;

CREATE TABLE IF NOT EXISTS room (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE,
    join_password VARCHAR(255) NOT NULL,
    admin_password VARCHAR(255) NOT NULL,
    deadline TIMESTAMP DEFAULT '2023-12-01 00:00:00',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS participant (
    id SERIAL PRIMARY KEY,
    room_id INTEGER REFERENCES room(id),
    email VARCHAR(255),
    name VARCHAR(255),
    participant_password VARCHAR(255) NOT NULL,
    assigned_to INTEGER REFERENCES participant(id) DEFAULT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (room_id, email),
    UNIQUE (room_id, name)
);

-- Insert test data for rooms
INSERT INTO room (name, join_password, admin_password)
VALUES 
    ('Room 1', 'join1', 'admin1'),
    ('Room 2', 'join2', 'admin2'),
    ('Room 3', 'join3', 'admin3');

-- Insert test data for participants in each room
-- Room 1 Participants
INSERT INTO participant (room_id, email, name, participant_password)
VALUES 
    (1, 'participant1a@example.com', 'Participant 1A', 'password1A'),
    (1, 'participant1b@example.com', 'Participant 1B', 'password1B'),
    (1, 'participant1c@example.com', 'Participant 1C', 'password1C'),
    (1, 'participant1d@example.com', 'Participant 1D', 'password1D');

-- Room 2 Participants
INSERT INTO participant (room_id, email, name, participant_password)
VALUES 
    (2, 'participant2a@example.com', 'Participant 2A', 'password2A'),
    (2, 'participant2b@example.com', 'Participant 2B', 'password2B'),
    (2, 'participant2c@example.com', 'Participant 2C', 'password2C'),
    (2, 'participant2d@example.com', 'Participant 2D', 'password2D');

-- Room 3 Participants
INSERT INTO participant (room_id, email, name, participant_password)
VALUES 
    (3, 'participant3a@example.com', 'Participant 3A', 'password3A'),
    (3, 'participant3b@example.com', 'Participant 3B', 'password3B'),
    (3, 'participant3c@example.com', 'Participant 3C', 'password3C'),
    (3, 'participant3d@example.com', 'Participant 3D', 'password3D');
`

func getPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = ":3000"
	} else {
		port = ":" + port
	}

	return port
}

func getEnvVar(name string) string {
	envVar, ok := os.LookupEnv(name) 
 
	if !ok {
		log.Fatalln("No such environment variable available: ", name)
	}

	return envVar
}

func hashPassword(password string) (string, error) {
    bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
    return string(bytes), err
}

func checkPasswordHash(password, hash string) bool {
    err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
    return err == nil
}

func main() {
	// Connect to the PostgreSQL database using the data source name
	connStr := getEnvVar("DATABASE_URL")
	db, err := sqlx.Connect("postgres", connStr)

    if err != nil {
		log.Fatalln(err)
	} else {
		defer db.Close()
	}

	// Ping the database to ensure it's reachable
	if err := db.Ping(); err != nil {
		log.Fatalln("Failed to ping the database:", err)
	} else {
		log.Println("Successfully connected to the database.")
	}
	
	db.MustExec(schema)
	

	log.Println("1")
	engine := html.New("./views", ".html")
	app := fiber.New(fiber.Config{Views: engine})
	
	log.Println("2")
	app.Use(csrf.New()) // Add CSRF middleware
	app.Use(limiter.New(limiter.Config{
        Max:        10, // max number of requests
        Expiration: 30 * time.Second, // time duration until the limit is reset
    }))

	log.Println("3")
	app.Get("/", func(c *fiber.Ctx) error {
		return c.Render("index",  fiber.Map{
            "Title": "Hello, World!",
        })
	})
	log.Println("4")
	
	err = app.Listen(getPort())
	if err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}

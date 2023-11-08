package main

import (
	"fmt"
    "strconv"
	"text/template"
    "bytes"
	"log"
	"os"
    "time"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
    "github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/template/html/v2"
	"golang.org/x/crypto/bcrypt"
)

const (
    doDevSetupDB = false // true for dev db setup
    schemaTemplate = `
        TRUNCATE TABLE room CASCADE;
        TRUNCATE TABLE participant CASCADE;

        DROP TABLE IF EXISTS participant;
        DROP TABLE IF EXISTS room;

        CREATE TABLE IF NOT EXISTS room (
            id SERIAL PRIMARY KEY,
            name VARCHAR(255) UNIQUE,
            join_password VARCHAR(255) NOT NULL,
            admin_password VARCHAR(255) NOT NULL,
            deadline TIMESTAMP DEFAULT '{{.DefaultDeadline}}',
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
        INSERT INTO participant (room_id, email, name, participant_password)
        VALUES 
            (1, 'participant1a@example.com', 'Participant 1A', 'password1A'),
            (1, 'participant1b@example.com', 'Participant 1B', 'password1B'),
            (1, 'participant1c@example.com', 'Participant 1C', 'password1C'),
            (1, 'participant1d@example.com', 'Participant 1D', 'password1D');

        INSERT INTO participant (room_id, email, name, participant_password)
        VALUES 
            (2, 'participant2a@example.com', 'Participant 2A', 'password2A'),
            (2, 'participant2b@example.com', 'Participant 2B', 'password2B'),
            (2, 'participant2c@example.com', 'Participant 2C', 'password2C'),
            (2, 'participant2d@example.com', 'Participant 2D', 'password2D');

        INSERT INTO participant (room_id, email, name, participant_password)
        VALUES 
            (3, 'participant3a@example.com', 'Participant 3A', 'password3A'),
            (3, 'participant3b@example.com', 'Participant 3B', 'password3B'),
            (3, 'participant3c@example.com', 'Participant 3C', 'password3C'),
            (3, 'participant3d@example.com', 'Participant 3D', 'password3D');
    `
)

/* 
    ##### Models
*/
type Room struct {
    ID            int       `db:"id"`
    Name          string    `db:"name"`
    JoinPassword  string    `db:"join_password"`
    AdminPassword string    `db:"admin_password"`
    Deadline      time.Time `db:"deadline"`
    CreatedAt     time.Time `db:"created_at"`
}

type Participant struct {
    ID                  int       `db:"id"`
    RoomID              int       `db:"room_id"`
    Email               string    `db:"email"`
    Name                string    `db:"name"`
    ParticipantPassword string    `db:"participant_password"`
    AssignedTo          *int      `db:"assigned_to"`  // nullable int
    CreatedAt           time.Time `db:"created_at"`
}

type RoomWithParticipantCount struct {
    Room
    ParticipantCount int `db:"participant_count"`
}

type CreateRoomFormData struct {
    RoomName       string `form:"roomName"`
    AdminPassword  string `form:"adminPassword"`
    JoinPassword   string `form:"joinPassword"`
    Deadline       string `form:"deadline"` 
}

type SignupParticipantData struct {
    Email               string    `form:"email"`
    Name                string    `form:"name"`
    ParticipantPassword string    `form:"participantPassword"`
}

/* 
    ##### Data Access Layer
*/
func dbSetupDatabaseSchema(db *sqlx.DB, config map[string]string) {
    tmpl, err := template.New("schema").Parse(schemaTemplate)
    if err != nil {
        log.Fatalf("Error parsing schema template: %v", err)
    }

    var schemaBuffer bytes.Buffer
    err = tmpl.Execute(&schemaBuffer, config)
    if err != nil {
        log.Fatalf("Error executing schema template: %v", err)
    }

    schema := schemaBuffer.String()
    db.MustExec(schema)
}

func dbGetAllRooms(db *sqlx.DB) ([]RoomWithParticipantCount, error) {
    var rooms []RoomWithParticipantCount
    query := `
    SELECT r.*, COUNT(p.id) as participant_count
    FROM room r
    LEFT JOIN participant p ON r.id = p.room_id
    GROUP BY r.id
	ORDER BY r.created_at DESC
    `
    err := db.Select(&rooms, query)
    return rooms, err
}

func dbGetOneRoom(db *sqlx.DB, roomId int) (Room, error) {
    var room Room
    query := `
    SELECT * 
    FROM room
    WHERE room.id = $1
    `
    err := db.Get(&room, query, roomId)
    return room, err
}

func dbGetParticipantsForRoom(db *sqlx.DB, roomId int) ([]Participant, error) {
    var participants []Participant
    query := `
    SELECT *
    FROM participant
    WHERE participant.room_id = $1
    `
    err := db.Select(&participants, query, roomId)
    return participants, err
}

func dbCreateNewRoom(db *sqlx.DB, data CreateRoomFormData) (int, error) {
	hashedAdminPassword, err := hashPassword(data.AdminPassword)
    if err != nil {
        return 0, err
    }

    hashedJoinPassword, err := hashPassword(data.JoinPassword)
    if err != nil {
        return 0, err
    }

	// Parse deadline
    deadline, err := time.Parse("2006-01-02T15:04", data.Deadline)
    if err != nil {
        return 0, err
    }

	// Prepare SQL query for inserting a new room
    query := `INSERT INTO room (name, join_password, admin_password, deadline) VALUES ($1, $2, $3, $4) RETURNING id`
    
	var roomId int
    err = db.QueryRow(query, data.RoomName, hashedJoinPassword, hashedAdminPassword, deadline).Scan(&roomId)
    if err != nil {
        return -1, err
    }

    return roomId, nil
}

/* 
    ##### Handlers
*/
func handleGetIndex(db *sqlx.DB) fiber.Handler {
    return func(c *fiber.Ctx) error {
        rooms, err := dbGetAllRooms(db)

        if err != nil {
            log.Println("Error fetching rooms:", err)
            return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
        }

        return c.Render("index", fiber.Map{
            "Title": "Secret Santa Rooms",
            "Rooms": rooms,
        })
    }
}


func handleGetCreateRoom(defaultDeadline string) fiber.Handler {
    return func(c *fiber.Ctx) error {
        return c.Render("create-room", fiber.Map{
            "Title": "Create Room - Secret Santa App",
            "DefaultDeadline": defaultDeadline,
        })
    }
}

func handleGetRoomDetails(db *sqlx.DB) fiber.Handler {
    return func(c *fiber.Ctx) error {
        roomId, err := strconv.Atoi(c.Params("id"))
        if err != nil {
            return c.Status(fiber.StatusBadRequest).SendString("Invalid room ID")
        }

        var room Room
        room, err = dbGetOneRoom(db, roomId)
        if err != nil {
            return c.Status(fiber.StatusBadRequest).SendString(fmt.Sprintf("Cannot get room with ID: %d. %s", roomId, err))
        }
        
        var participants []Participant
        participants, err = dbGetParticipantsForRoom(db, roomId)
        if err != nil {
            return c.Status(fiber.StatusBadRequest).SendString(fmt.Sprintf("Cannot get participants for room ID: %d. %s", roomId, err))
        }

        return c.Render("room-details", fiber.Map{
            "Room":         room,
            "Participants": participants,
        })
    }
}

func handlePostCreateRoom(db *sqlx.DB) fiber.Handler {
    return func(c *fiber.Ctx) error {
        var data CreateRoomFormData
        if err := c.BodyParser(&data); err != nil {
            log.Println("Error parsing form:", err)
            return c.Status(fiber.StatusBadRequest).SendString(fmt.Sprintf("Error parsing form data: %s", err))
        }

        roomId, err := dbCreateNewRoom(db, data)
        if err != nil {
            // Handle error appropriately
            return c.Status(fiber.StatusInternalServerError).SendString(fmt.Sprintf("Error creating room: %s", err))
        }

        return c.Redirect(fmt.Sprintf("/room-details/%d", roomId))
    }
}

func handlePostJoinRoom(c *fiber.Ctx) error {
    return nil
}

/* 
    ##### Utils
*/
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
    // Gen ENV vars
	connStr := getEnvVar("DATABASE_URL")
	defaultDeadline := os.Getenv("DEFAULT_DEADLINE")
    if defaultDeadline == "" {
		defaultDeadline = "2022-12-01 00:00:00"	
    }

    // Create config map 
    config := map[string]string{
        "DefaultDeadline": defaultDeadline,
    }

	// Connect to PostgreSQL
	db, err := sqlx.Connect("postgres", connStr)
    defer db.Close()

    if err != nil {
		log.Fatalln(err)
	} 

    // Create initial DB schema 
    if doDevSetupDB {
        dbSetupDatabaseSchema(db, config)	
    }
	
	// Set up Fiber
	engine := html.New("./views", ".html")
    app := fiber.New(fiber.Config{Views: engine})
    app.Use(logger.New())
    app.Use(limiter.New(limiter.Config{
        Max:        100, 
        Expiration: 30 * time.Second, 
    }))

	// Set up routes
	app.Get("/", handleGetIndex(db))
    app.Get("/room-details/:id", handleGetRoomDetails(db))
	app.Get("/create-room", handleGetCreateRoom(defaultDeadline))
    app.Post("/create-room", handlePostCreateRoom(db))

    // Run server
	err = app.Listen(getPort())
	if err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}

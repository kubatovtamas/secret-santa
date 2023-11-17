package main

import (
    "errors"
    mRand "math/rand"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/gofiber/template/html/v2"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"github.com/sendgrid/sendgrid-go"
    "github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/robfig/cron/v3"
	"io"
	"log"
	"os"
	"strconv"
	"text/template"
	"time"
)

const (
	doDevSetupDB   = false 
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
			draw_completed BOOL NOT NULL DEFAULT FALSE,
            deadline TIMESTAMP DEFAULT '{{.DefaultDeadline}}',
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );

        CREATE TABLE IF NOT EXISTS participant (
            id SERIAL PRIMARY KEY,
            room_id INTEGER REFERENCES room(id),
            email VARCHAR(255),
            name VARCHAR(255),
            participant_password VARCHAR(255) NOT NULL,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            UNIQUE (room_id, email),
            UNIQUE (room_id, name)
        );
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
	DrawCompleted bool      `db:"draw_completed"`
	Deadline      time.Time `db:"deadline"`
	CreatedAt     time.Time `db:"created_at"`
}

type Participant struct {
	ID                  int       `db:"id"`
	RoomID              int       `db:"room_id"`
	Email               string    `db:"email"`
	Name                string    `db:"name"`
	ParticipantPassword string    `db:"participant_password"`
	CreatedAt           time.Time `db:"created_at"`
}

type RoomWithParticipantCount struct {
	Room
	ParticipantCount int `db:"participant_count"`
}

type CreateRoomFormData struct {
	RoomName      string `form:"roomName"`
	AdminPassword string `form:"adminPassword"`
	JoinPassword  string `form:"joinPassword"`
	Deadline      string `form:"deadline"`
}

type CreateParticipantFormData struct {
	Email               string `form:"email"`
	Name                string `form:"name"`
	ParticipantPassword string `form:"participantPassword"`
}

type Assignment struct {
	Participant      Participant
	GifteeName       string
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
	hashedAdminPassword, err := hashString(data.AdminPassword)
	if err != nil {
		return -1, err
	}

	hashedJoinPassword, err := hashString(data.JoinPassword)
	if err != nil {
		return -1, err
	}

	deadline, err := time.Parse("2006-01-02T15:04", data.Deadline)
	if err != nil {
		return -1, err
	}

	query := `INSERT INTO room (name, join_password, admin_password, deadline) VALUES ($1, $2, $3, $4) RETURNING id`

	var roomId int
	err = db.QueryRow(query, data.RoomName, hashedJoinPassword, hashedAdminPassword, deadline).Scan(&roomId)
	if err != nil {
		return -1, err
	}

	return roomId, nil
}

func dbCreateNewParticipant(db *sqlx.DB, data CreateParticipantFormData, roomId int, encryptionKey []byte) (int, error) {
	hashedParticipantPassword, err := hashString(data.ParticipantPassword)
	if err != nil {
		return -1, err
	}

	encryptedEmail, err := encryptAES(encryptionKey, data.Email)
	if err != nil {
		return -1, err
	}

	var participantId int
	query := `
    INSERT INTO 
    participant (
        room_id,
        email,
        name,
        participant_password
    )
    VALUES ($1, $2, $3, $4)
    RETURNING id
    `
	err = db.QueryRow(query, roomId, encryptedEmail, data.Name, hashedParticipantPassword).Scan(&participantId)
	if err != nil {
		return -1, err
	}

	return participantId, nil
}

func dbGetJoinPasswordForRoom(db *sqlx.DB, roomId int) (string, error) {
	var storedPasswordHash string
	query := `
    SELECT join_password
    FROM room
    WHERE id = $1
    `

	err := db.Get(&storedPasswordHash, query, roomId)
	if err != nil {
		return "", nil
	}

	return storedPasswordHash, nil
}

func dbSetRoomToDrawCompleted(db *sqlx.DB, roomId int) error {
	query := `
	UPDATE room
	SET draw_completed = TRUE
	WHERE id = $1
	`

	_, err := db.Exec(query, roomId)

	return err
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
			"Title": "Titkowos Mikuwulás Főoldal",
			"Rooms": rooms,
		})
	}
}

func handleGetCreateRoom(defaultDeadline string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		return c.Render("create-room", fiber.Map{
			"DefaultDeadline": defaultDeadline,
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

		log.Println("Created new room with ID:", roomId)
		return c.Redirect(fmt.Sprintf("/room-details/%d", roomId))
	}
}

func handleGetRoomDetails(db *sqlx.DB, store *session.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
		roomId, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid room ID")
		}

		sess, err := store.Get(c)
		if err != nil || sess.Get("roomAccess") != roomId {
			return c.Redirect("/")
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

func handlePostRoomDetails(db *sqlx.DB, store *session.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
		roomId, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid room ID")
		}

		joinPassword := c.FormValue("joinPassword")
		hashedJoinPassword, err := dbGetJoinPasswordForRoom(db, roomId) // Implement this function
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString(fmt.Sprintf("Error joining room, roomId=%d, err=%s", roomId, err))
		}

		if checkStringHash(joinPassword, hashedJoinPassword) {
			sess, _ := store.Get(c)
			sess.Set("roomAccess", roomId)
			sess.Save()
			return c.Redirect(fmt.Sprintf("/room-details/%d", roomId))
		} else {
			return c.Redirect("/")
		}
	}
}

func handleGetJoinRoom() fiber.Handler {
	return func(c *fiber.Ctx) error {
		roomId, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid room ID")
		}

		return c.Render("join-room", fiber.Map{
			"Title":  "Join Room - Secret Santa App",
			"roomId": roomId,
		})
	}
}

func handlePostJoinRoom(db *sqlx.DB, encryptionKey []byte) fiber.Handler {
	return func(c *fiber.Ctx) error {
		roomId, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid room ID")
		}

		var data CreateParticipantFormData
		if err := c.BodyParser(&data); err != nil {
			log.Println("Error parsing form:", err)
			return c.Status(fiber.StatusBadRequest).SendString(fmt.Sprintf("Error parsing form data: %s", err))
		}
		// log.Println(data)
		participantId, err := dbCreateNewParticipant(db, data, roomId, encryptionKey)
		if err != nil {
			// Handle error appropriately
			return c.Status(fiber.StatusInternalServerError).SendString(fmt.Sprintf("Error adding participant: %s", err))
		}

		log.Println("Created new participant with ID:", participantId, "for room with ID:", roomId)
		return c.Redirect(fmt.Sprintf("/room-details/%d", roomId))
	}
}

/*
   ##### Utils
*/
func AssignSecretSanta(participants []Participant) ([]Assignment, error) {
    if len(participants) < 2 {
        return nil, errors.New("a minimum of 2 participants is required")
    }

    // Create a list of Assignments
    assignments := make([]Assignment, len(participants))
    for i, p := range participants {
        assignments[i] = Assignment{
            Participant: p, 
            GifteeName: p.Name, 
        }
    }

    // Randomly shuffle the list.
    mRand.Seed(time.Now().UnixNano())
    mRand.Shuffle(len(assignments), func(i, j int) { 
        assignments[i], assignments[j] = assignments[j], assignments[i] 
    })

    // Shift the giftees by one
    for i := 0; i < len(assignments); i++ {
        gifteeIdx := (i + 1) % len(assignments)
        assignments[i].GifteeName = assignments[gifteeIdx].Participant.Name
    }

    return assignments, nil
}

func sendEmail(assignment Assignment) error {
    // Sender and recipient information
    from := mail.NewEmail("Raul", getEnvVar("SENDGRID_EMAIL_FROM"))
    to := mail.NewEmail(assignment.Participant.Name, assignment.Participant.Email)

    // Create a new SendGrid message
    message := mail.NewV3Mail()

    // Set the 'from' address
    message.SetFrom(from)

    // Create a personalization object
    p := mail.NewPersonalization()
    p.AddTos(to)

    // Set dynamic template data based on your template placeholders
    p.SetDynamicTemplateData("Name", assignment.Participant.Name)
    p.SetDynamicTemplateData("Giftee", assignment.GifteeName)

    // Add personalization to the message
    message.AddPersonalizations(p)

    // Set the Template ID from SendGrid
    templateID := getEnvVar("SENDGRID_TEMPLATE_ID")
    message.SetTemplateID(templateID)

    // Create a SendGrid client and send the message
    client := sendgrid.NewSendClient(getEnvVar("SENDGRID_API_KEY"))
    response, err := client.Send(message)
    if err != nil {
        return err
    }

    log.Println("Email Sent to:", assignment.Participant.Name, "Status Code:", response.StatusCode)
    return nil
}

func startScheduler(db *sqlx.DB, encryptionKey []byte) {
    c := cron.New()
    c.AddFunc("0 * * * *", func() {
        now := time.Now().UTC().Add(time.Hour)
		log.Println("Scheduler run:", now)
        rooms, err := dbGetAllRooms(db)
		log.Println("Lenrooms:", len(rooms))
        if err != nil {
            log.Printf("Error fetching rooms for draw: %s", err)
            return
        }

        for _, room := range rooms {
			log.Println("Room:", room.Name, "Deadline:", room.Deadline, "Completed:", room.DrawCompleted)
            if now.After(room.Deadline) && !room.DrawCompleted { 
                log.Printf("Processing draw for room: %d", room.ID)
                
                // Fetch participants from database
                participants, err := dbGetParticipantsForRoom(db, room.ID)
                if err != nil {
                    log.Printf("Error fetching participants for room %d: %s", room.ID, err)
                    continue
                }
                log.Println("Fetched participants for the draw")

                // Decrypt participant emails
                for i := range participants {
                    decryptedEmail, err := decryptAES(encryptionKey, participants[i].Email)
                    if err != nil {
                        log.Printf("Error decrypting email for participant %d: %s", participants[i].ID, err)
                        continue
                    }
                    participants[i].Email = decryptedEmail
                }
                log.Println("Decrypted participant emails")

                // Assign Secret Santa
                assignments, err := AssignSecretSanta(participants)
                if err != nil {
                    log.Printf("Error in assigning Secret Santa for room %d: %s", room.ID, err)
                    continue
                }
                log.Println("Secret Santa assigned")

                // Send emails
                for _, assignment := range assignments {
                    err := sendEmail(assignment)
                    if err != nil {
                        log.Printf("Failed to send email to %s: %s", assignment.Participant.Email, err)
                    }
                }
                log.Println("Emails sent for the draw")

                // Update room to indicate draw is completed
				dbSetRoomToDrawCompleted(db, room.ID)
            }
        }
    })
    c.Start()
}

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

func hashString(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func checkStringHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func decodeEncryptionKey(encryptionKey string) []byte {
	decodedKey, err := base64.StdEncoding.DecodeString(encryptionKey)
	if err != nil {
		log.Fatalln("Failed to decode encryption key:", err)
	}

	return decodedKey
}

func encryptAES(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

func decryptAES(key []byte, ciphertext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	enc, err := hex.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	nonceSize := aesGCM.NonceSize()
	if len(enc) < nonceSize {
		return "", err
	}

	nonce, ciphertextBytes := enc[:nonceSize], enc[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

/*
   ##### Main
*/
func main() {
	// Gen ENV vars
	connStr := getEnvVar("DATABASE_URL")
	encodedEncryptionKey := getEnvVar("ENCRYPTION_KEY")
	defaultDeadline := getEnvVar("DEFAULT_DEADLINE")

	decodedEncryptionKey := decodeEncryptionKey(encodedEncryptionKey)

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
	var store *session.Store
	store = session.New()
	app := fiber.New(fiber.Config{Views: engine})

	app.Use(logger.New())
	app.Use(limiter.New(limiter.Config{
		Max:        100,
		Expiration: 30 * time.Second,
	}))

	// Set up routes
	app.Get("/", handleGetIndex(db))

	app.Get("/room-details/:id", handleGetRoomDetails(db, store))
	app.Post("/room-details/:id", handlePostRoomDetails(db, store))

	app.Get("/create-room", handleGetCreateRoom(defaultDeadline))
	app.Post("/create-room", handlePostCreateRoom(db))

	app.Get("/room-details/:id/join-room", handleGetJoinRoom())
	app.Post("/room-details/:id/join-room", handlePostJoinRoom(db, decodedEncryptionKey))

	// Start the scheduler
    startScheduler(db, decodedEncryptionKey)

	// Run server
	err = app.Listen(getPort())
	if err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}

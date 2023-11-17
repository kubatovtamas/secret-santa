// +heroku goVersion 1.16

module secret-santa

go 1.16

require (
	github.com/gofiber/fiber/v2 v2.50.0
	github.com/gofiber/template/html/v2 v2.0.5
	github.com/jmoiron/sqlx v1.3.5
	github.com/lib/pq v1.2.0
	github.com/robfig/cron/v3 v3.0.1
	github.com/sendgrid/rest v2.6.9+incompatible // indirect
	github.com/sendgrid/sendgrid-go v3.13.0+incompatible
	github.com/stretchr/testify v1.8.4 // indirect
	golang.org/x/crypto v0.7.0
)

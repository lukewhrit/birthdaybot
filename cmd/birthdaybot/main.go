package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/caarlos0/env"
	"github.com/go-co-op/gocron"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

type Options = *discordgo.ApplicationCommandInteractionDataOption

var (
	s      *discordgo.Session
	logger = log.New(os.Stderr, "bot ", log.LstdFlags)
	db     *sqlx.DB
	c      = gocron.NewScheduler(time.UTC)

	config struct {
		Token        string `env:"TOKEN"`
		DSN          string `env:"DSN"`
		BirthdayRole string `env:"BIRTHDAY_ROLE"`
	}

	months = map[time.Month]string{
		1:  "January",
		2:  "Feburary",
		3:  "March",
		4:  "April",
		5:  "May",
		6:  "June",
		7:  "July",
		8:  "August",
		9:  "September",
		10: "October",
		11: "Noveember",
		12: "December",
	}

	schema = `
CREATE TABLE users (
	id text PRIMARY KEY,
	birthdate integer
) STRICT;`
)

type User struct {
	ID       string `db:"id"`
	Birthday int64  `db:"birthdate"`
}

func init() {
	if err := env.Parse(&config); err != nil {
		logger.Fatalf("Failed to load config: %v", err)
	}

	var dbErr error
	db, dbErr = sqlx.Connect("sqlite3", config.DSN)

	if dbErr != nil {
		logger.Fatalf("DB connection failed: %v", dbErr)
	}

	db.Exec(schema)

	var dgErr error
	s, dgErr = discordgo.New("Bot " + config.Token)

	if dgErr != nil {
		logger.Fatalf("Invalid bot parameters: %v", dgErr)
	}
}

var (
	commands = []*discordgo.ApplicationCommand{
		{
			Name:        "set-birthday",
			Description: "Basic Command",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "date",
					Description: "Your birthday! In MMM-DD (e.g. Jun 06), please!",
					Type:        discordgo.ApplicationCommandOptionString,
				},
			},
		},
	}

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"set-birthday": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: (func() string {
						options := parseOptions(i.ApplicationCommandData().Options)

						oldDate, err := time.Parse("Jan 02", options["date"].StringValue())

						if err != nil {
							return fmt.Sprintf("There was an error responding to your command: ```%v```", err)
						}

						date := time.Date(1970, oldDate.Month(), oldDate.Day(), oldDate.Hour(),
							oldDate.Minute(), oldDate.Second(), oldDate.Nanosecond(),
							oldDate.Location())
						tx, err := db.Begin()

						if err != nil {
							return fmt.Sprintf("There was an error responding to your command: ```%v```", err)
						}

						fmt.Println(date.UTC().Unix(), date.Unix())

						if _, err := tx.Exec("INSERT INTO users (id, birthdate) VALUES ($1, $2) ON CONFLICT (id) DO UPDATE SET birthdate = $2", i.Member.User.ID, date.UTC().Unix()); err != nil {
							return fmt.Sprintf("There was an error responding to your command: ```%v```", err)
						}

						if err := tx.Commit(); err != nil {
							return fmt.Sprintf("There was an error responding to your command: ```%v```", err)
						}

						return fmt.Sprintf(
							"Success! %s, you have set your birthday to %s %v%s.",
							i.Member.Mention(),
							months[date.Month()],
							date.Day(),
							ord(date.Day()),
						)
					})(),
				},
			})
		},
	}
)

func init() {
	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	})
}

func main() {
	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		logger.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
	})

	s.Identify.Intents = discordgo.IntentGuildMessages

	err := s.Open()

	if err != nil {
		logger.Fatalf("Cannot open session: %s", err)
	}

	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))

	for i, v := range commands {
		cmd, err := s.ApplicationCommandCreate(s.State.User.ID, "", v)

		if err != nil {
			logger.Panicf("Cannot create '%v' command: %v", v.Name, err)
		}
		registeredCommands[i] = cmd
	}

	users := []User{}
	if err := db.Select(&users, "SELECT * FROM users WHERE strftime('%m-%d', birthdate, 'unixepoch') = strftime('%m-%d', 'now', 'utc');"); err != nil {
		logger.Fatalf("Error accessing from database: %v", err)
	}

	fmt.Println(users)

	for _, user := range users {
		fmt.Println(user)
	}

	c.Cron("0 0 * * *").Do(func() {
		users := []User{}
		if err := db.Select(&users, "SELECT * FROM users WHERE strftime('%m-%d', birthdate, 'unixepoch') = strftime('%m-%d', 'now', 'utc');"); err != nil {
			logger.Fatalf("Error accessing from database: %v", err)
		}

		fmt.Println(users)

		for _, user := range users {
			fmt.Println(user)
		}
	})

	defer s.Close()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	logger.Println("Press Ctrl+C to exit")
	<-stop

	for _, v := range registeredCommands {
		err := s.ApplicationCommandDelete(s.State.User.ID, "", v.ID)
		if err != nil {
			logger.Panicf("Cannot delete '%v' command: %v", v.Name, err)
		}
	}

	logger.Println("Gracefully shutting down...")
}

func ord(i int) string {
	switch i {
	case 1:
		return "st"
	case 2:
		return "nd"
	default:
		return "th"
	}
}

func parseOptions(raw []Options) map[string]Options {
	resp := make(map[string]Options, len(raw))

	for _, opt := range raw {
		resp[opt.Name] = opt
	}

	return resp
}

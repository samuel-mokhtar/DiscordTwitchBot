package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/samuel-mokhtar/DiscordTwitchBot/handlers"
	"github.com/samuel-mokhtar/DiscordTwitchBot/twitch"
	"github.com/samuel-mokhtar/DiscordTwitchBot/utils"
)

// Variables used for command line parameters
var (
	token     string
	tokenPath string
)

func init() {
	flag.StringVar(&token, "t", "", "Bot Token")
	flag.StringVar(&tokenPath, "p", "", "Path to Bot Token")
	flag.Parse()

	// We process the most important flag to receive a token
	// The flags listed in order of importance are
	// t > p
	// If no flags are set the Bot loads token from environment variable BOT_TOKEN
	if len(token) > 0 {

	} else if len(tokenPath) > 0 {
		rawToken, err := os.ReadFile(tokenPath)
		if err != nil {
			utils.Log.WithError(err).Fatal("Token file could not be read")
		}
		token = string(rawToken)
	} else {
		utils.Log.Warning("No Flags specified. Loading bot token from the environment variable BOT_TOKEN.")
		token = os.Getenv("BOT_TOKEN")
	}
}

func main() {
	// Create a new Discord session using the provided bot token.
	dg, errDiscord := discordgo.New("Bot " + token)
	if errDiscord != nil {
		utils.Log.WithError(errDiscord).Fatal("Discord session could not be created.")
	}

	// Create a new Twitch session with client id, secret, and a path to saved data
	ts, errTwitch := twitch.New(os.Getenv("TWITCH_CLIENT_ID"), os.Getenv("TWITCH_CLIENT_SECRET"), "session1")
	if errTwitch != nil {
		utils.Log.WithError(errTwitch).Error("Twitch session could not be created.")
	}

	utils.Log.Info("Bot is starting up.")

	// Register event handlers
	dg.AddHandler(handlers.GuildCreate)
	dg.AddHandler(handlers.GuildDelete)
	dg.AddHandler(handlers.MessageCreate)

	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages

	// Open a websocket connection to Discord and begin listening.
	errDiscord = dg.Open()
	if errDiscord != nil {
		utils.Log.WithError(errDiscord).Fatal("Could not establish connection to Discord.")
	}

	// Open a connection to twitch
	utils.Log.Info("Establishing connection to Twitch.")
	errTwitch = ts.GetAuthToken()
	if errTwitch != nil {
		utils.Log.WithError(errTwitch).Error("Could not establish connection to Twitch.")
	}

	// Start monitoring Twitch
	go twitch.StartMonitoring(ts, dg)

	// Wait here until CTRL-C or other term signal is received.
	utils.Log.Info("Bot is now running.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Cleanly shut down the Twitch session
	utils.Log.Info("Twitch session is shutting down.")
	ts.Close()

	// Cleanly close down the Discord session.
	utils.Log.Info("Bot is shutting down.")
	dg.Close()

	utils.Log.Info("Bot has shutdown.")
}

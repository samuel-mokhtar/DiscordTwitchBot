package handlers

import (
	"github.com/SamIAm2718/HouseDiscordBot/twitch"
	"github.com/SamIAm2718/HouseDiscordBot/utils"
	"github.com/bwmarrin/discordgo"
)

func GuildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {
	if event.Guild.Unavailable {
		utils.Log.Debugf("Guild %v is unavailable.\n", event.ID)
		twitch.SetGuildUnavailable(event.ID)
		return
	}

	utils.Log.Debugf("Connected to guild %v.\n", event.ID)
	twitch.SetGuildActive(event.ID)
}

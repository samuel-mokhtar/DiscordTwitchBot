package handlers

import (
	"strings"

	"github.com/SamIAm2718/HouseDiscordBot/twitch"
	"github.com/SamIAm2718/HouseDiscordBot/utils"
	"github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
)

func MessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	if strings.HasPrefix(strings.ToLower(m.Content), "housebot") {

		utils.Log.WithFields(logrus.Fields{
			"user":       m.Author.Username,
			"command":    m.Content,
			"channel_id": m.ChannelID,
			"server_id":  m.GuildID}).Info("Command recieved.")

		commandParams := strings.Split(m.Content, " ")[1:]

		if len(commandParams) > 0 {
			switch commandParams[0] {
			case "channel":
				commandChannel(s, m, commandParams[1:])
				return
			}
		}

		utils.Log.WithFields(logrus.Fields{
			"user":       m.Author.Username,
			"command":    m.Content,
			"channel_id": m.ChannelID,
			"server_id":  m.GuildID}).Info("Invalid command.")
	}
}

func commandChannel(s *discordgo.Session, m *discordgo.MessageCreate, c []string) {
	if len(c) == 2 {
		switch c[0] {
		case "add":
			t := twitch.GetSession(s)
			twitchChannel := strings.ToLower(c[1])

			if t.RegisterChannel(twitchChannel, m.GuildID, m.ChannelID) {
				utils.Log.WithFields(logrus.Fields{
					"user":           m.Author.Username,
					"twitch_channel": twitchChannel,
					"channel_id":     m.ChannelID,
					"server_id":      m.GuildID}).Info("Succeeded in registering channel.")

				_, err := s.ChannelMessageSend(m.ChannelID, twitchChannel+"'s Twitch channel successfully added to this Discord channel.")
				if err != nil {
					utils.Log.WithFields(logrus.Fields{"error": err}).Error("Failed to send message to Discord.")
				}
			} else {
				utils.Log.WithFields(logrus.Fields{
					"user":           m.Author.Username,
					"twitch_channel": twitchChannel,
					"channel_id":     m.ChannelID,
					"server_id":      m.GuildID}).Info("Failed to register channel.")

				_, err := s.ChannelMessageSend(m.ChannelID, twitchChannel+"'s Twitch channel is already added to this Discord channel.")
				if err != nil {
					utils.Log.WithFields(logrus.Fields{"error": err}).Error("Failed to send message to Discord.")
				}
			}
			return
		case "remove":
			t := twitch.GetSession(s)
			twitchChannel := strings.ToLower(c[1])

			if t.UnregisterChannel(twitchChannel, m.GuildID, m.ChannelID) {
				utils.Log.WithFields(logrus.Fields{
					"user":           m.Author.Username,
					"twitch_channel": twitchChannel,
					"channel_id":     m.ChannelID,
					"server_id":      m.GuildID}).Info("Succeeded in unregistering channel.")

				_, err := s.ChannelMessageSend(m.ChannelID, twitchChannel+"'s Twitch channel successfully removed from this Discord channel.")
				if err != nil {
					utils.Log.WithFields(logrus.Fields{"error": err}).Error("Failed to send message to Discord.")
				}
			} else {
				utils.Log.WithFields(logrus.Fields{
					"user":           m.Author.Username,
					"twitch_channel": twitchChannel,
					"channel_id":     m.ChannelID,
					"server_id":      m.GuildID}).Info("Failed to unregister channel.")

				_, err := s.ChannelMessageSend(m.ChannelID, twitchChannel+"'s Twitch channel is not added to this Discord channel.")
				if err != nil {
					utils.Log.WithFields(logrus.Fields{"error": err}).Error("Failed to send message to Discord.")
				}

			}
			return
		default:
		}
	}

	_, err := s.ChannelMessageSend(m.ChannelID, "Proper usage is housebot channel [add/remove] <Twitch Channel>")
	if err != nil {
		utils.Log.WithFields(logrus.Fields{"error": err}).Error("Failed to send message to Discord.")
	}
}

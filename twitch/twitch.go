package twitch

import (
	"encoding/gob"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/SamIAm2718/HouseDiscordBot/constants"
	"github.com/SamIAm2718/HouseDiscordBot/utils"
	"github.com/bwmarrin/discordgo"
	"github.com/nicklaw5/helix"
)

type discordChannel struct {
	ChannelID            string // ID of discord channel
	LiveNotificationSent bool   // Whether or not a channel was notified of being live
}

type twitchChannelInfo struct {
	DisplayName     string                       // Twitch display name
	LogoURL         string                       // URL of Twitch logo
	StreamTitle     string                       // Title of stream
	GameID          string                       // Game title being streamed
	ThumbnailURL    string                       // URL of thumbnail
	StartTime       time.Time                    // Start time of stream
	EndTime         time.Time                    // End time of stream
	DiscordChannels map[string][]*discordChannel // Map of Discord guild IDs to discordChannel
}

type Session struct {
	name        string                        // Name of the Twitch session
	client      *helix.Client                 // Helix client for sending HTTP requests to twitch
	isConnected bool                          // Status of Helix client connection to twitch
	twitchData  map[string]*twitchChannelInfo // Map of twitch channel to its info
}

var (
	activeSessions map[string]*Session // Map of Discord sessions to twitch sessions
	guildStatus    map[string]bool     // Map of Guild ID to status of guild connection
)

func init() {
	activeSessions = make(map[string]*Session)
	guildStatus = make(map[string]bool)
}

func (t *Session) Close() error {
	t.isConnected = false

	for _, tcInfo := range t.twitchData {
		for gID, status := range guildStatus {
			if !status {
				delete(tcInfo.DiscordChannels, gID)
			}
		}
	}

	return utils.WriteGobToDisk(constants.DataPath, t.name, t.twitchData)
}

func GetSession(s *discordgo.Session) *Session {
	return activeSessions[s.State.SessionID]
}

func New(id string, secret string, name string) (t *Session, err error) {
	t = &Session{}
	t.name = name

	t.client, err = helix.NewClient(&helix.Options{
		ClientID:     id,
		ClientSecret: secret,
		RedirectURI:  "http://localhost",
	})
	if err != nil {
		return t, err
	}

	t.twitchData = make(map[string]*twitchChannelInfo)

	err = readGobFromDisk(constants.DataPath, t.name, &t.twitchData)
	if errors.Is(err, os.ErrNotExist) {
		utils.Log.Warn("Twitch session info does not exist on disk. Will be created on shutdown.")
		err = nil
	}

	return t, err
}

// Attempts to use client ID and secret to get Auth token from twitch.
// If successful then set the session state to connected.
func (t *Session) GetAuthToken() error {
	resp, err := t.client.RequestAppAccessToken([]string{""})
	if err != nil {
		return err
	} else if resp.Data.AccessToken == "" {
		return constants.ErrEmptyAccessToken
	}
	t.client.SetAppAccessToken(resp.Data.AccessToken)
	t.isConnected = true

	return nil
}

// Registers a Discord Channel to monitor the live state of a twitch channel
func (t *Session) RegisterChannel(twitchID string, discordGuildID string, discordChannelID string) (registered error) {
	// if twitch channel doesn't exist, register as new channel
	if t.twitchData[twitchID] == nil {

		// we need to obtain the profile picture url and display name for the twitch channel
		if validateAndRefreshAuthToken(t) {
			resp, err := t.client.GetUsers(&helix.UsersParams{Logins: []string{twitchID}})
			if err != nil {
				utils.Log.WithError(err).Error("Failed to query twitch.")
			}

			if len(resp.Data.Users) == 0 {
				return constants.ErrTwitchUserDoesNotExist
			}

			// register the twitch information channel
			t.twitchData[twitchID] = &twitchChannelInfo{
				DisplayName:     resp.Data.Users[0].DisplayName,
				LogoURL:         resp.Data.Users[0].ProfileImageURL,
				DiscordChannels: make(map[string][]*discordChannel),
			}
		} else {
			return constants.ErrInvalidToken
		}
	}

	// check if twitch session contains discord oracle, register otherwise
	if t.getChannelIdx(twitchID, discordGuildID, discordChannelID) < 0 {
		dc := &discordChannel{
			ChannelID:            discordChannelID,
			LiveNotificationSent: false,
		}
		t.twitchData[twitchID].DiscordChannels[discordGuildID] = append(t.twitchData[twitchID].DiscordChannels[discordGuildID], dc)

		// Writes the data to the disk in case of crash
		if err := utils.WriteGobToDisk(constants.DataPath, t.name, t.twitchData); err != nil {
			utils.Log.WithError(err).Error("Error writing data to disk.")
		}

		return nil
	}

	return constants.ErrTwitchUserRegistered
}

// Sets the current guild as active
func SetGuildActive(guildID string) {
	guildStatus[guildID] = true
}

// Sets the current guild as inactive
func SetGuildInactive(guildID string) {
	guildStatus[guildID] = false
}

// Sets current guild as unavailable
func SetGuildUnavailable(guildID string) {
	delete(guildStatus, guildID)
}

// Adds session to activeSessions if it is connected to Twitch and begins to monitor Twitch
func StartMonitoring(t *Session, s *discordgo.Session) {
	if t.isConnected {
		activeSessions[s.State.SessionID] = t

		go monitorChannels(t, s)
	}
}

// Unregisters a Discord Channel from monitor the live state of a Twitch channel
func (t *Session) UnregisterChannel(twitchID string, discordGuildID string, discordChannelID string) (unregistered bool) {
	if channelIdx := t.getChannelIdx(twitchID, discordGuildID, discordChannelID); channelIdx >= 0 {
		t.twitchData[twitchID].DiscordChannels[discordGuildID] = remove(t.twitchData[twitchID].DiscordChannels[discordGuildID], channelIdx)

		// Check if no more channels in Discord server are monitoring for Twitch channel and if so delete from map
		if len(t.twitchData[twitchID].DiscordChannels[discordGuildID]) == 0 {
			delete(t.twitchData[twitchID].DiscordChannels, discordGuildID)
		}

		// check if Oracles are empty and if so, delete channel from twitch Session
		if len(t.twitchData[twitchID].DiscordChannels) == 0 {
			utils.Log.Debugf("No more channels monitoring for %v. Deleting Twitch info for %v.\n", twitchID, twitchID)
			delete(t.twitchData, twitchID)
		}

		// Writes the data to the disk in case of crash
		if err := utils.WriteGobToDisk(constants.DataPath, t.name, t.twitchData); err != nil {
			utils.Log.WithError(err).Error("Error writing data to disk.")
		}

		return true
	}
	return false
}

func createDiscordEmbedMessage(t *twitchChannelInfo) *discordgo.MessageEmbed {
	embed := discordgo.MessageEmbed{
		URL:         "https://www.twitch.tv/" + t.DisplayName,
		Type:        "",
		Title:       t.StreamTitle,
		Description: "",
		Timestamp:   "",
		Color:       0x808080,
		Footer:      &discordgo.MessageEmbedFooter{},
		Image: &discordgo.MessageEmbedImage{
			URL:      strings.Replace(strings.Replace(t.ThumbnailURL, "{width}", "1920", -1), "{height}", "1080", -1),
			ProxyURL: "",
			Width:    1920,
			Height:   1080},
		Thumbnail: &discordgo.MessageEmbedThumbnail{},
		Video:     &discordgo.MessageEmbedVideo{},
		Provider:  &discordgo.MessageEmbedProvider{},
		Author: &discordgo.MessageEmbedAuthor{
			Name:    t.DisplayName,
			IconURL: t.LogoURL,
		},
		Fields: []*discordgo.MessageEmbedField{},
	}

	return &embed
}

// Returns -1 if oracle isn't present or the index of the oracle if it is
func (t *Session) getChannelIdx(twitchID string, discordGuildID string, discordChannelID string) int {
	if t.twitchData[twitchID] == nil {
		return -1
	}
	for i, d := range t.twitchData[twitchID].DiscordChannels[discordGuildID] {
		if d.ChannelID == discordChannelID {
			return i
		}
	}
	return -1
}

func monitorChannels(ts *Session, ds *discordgo.Session) {
	for ts.isConnected {
		if validateAndRefreshAuthToken(ts) {
			var queryChannels []string

			for twitchChannel := range ts.twitchData {
				queryChannels = append(queryChannels, twitchChannel)
			}

			resp, err := ts.client.GetStreams(&helix.StreamsParams{
				UserLogins: queryChannels,
			})
			if err != nil {
				utils.Log.WithError(err).Error("Failed to query twitch.")
			}

			if constants.DebugTwitchResponse {
				empJSON, err := json.MarshalIndent(resp, "", "  ")
				if err != nil {
					utils.Log.WithError(err).Debug("Error marshaling Twitch JSON response.")
				} else {
					utils.Log.Debugf("Twitch getStreams request Response: %+v\n", string(empJSON))
				}
			}

			// populate start/end time
		OUTER:
			for twitchChannel, tcInfo := range ts.twitchData {
				for _, streams := range resp.Data.Streams {
					if streams.UserLogin == twitchChannel && streams.Type == "live" {
						tcInfo.StreamTitle = streams.Title
						tcInfo.GameID = streams.GameID
						tcInfo.ThumbnailURL = streams.ThumbnailURL
						tcInfo.StartTime = streams.StartedAt
						tcInfo.EndTime = time.Time{}
						continue OUTER
					}
				}

				// stream not found, update times
				tcInfo.StartTime = time.Time{}
				if tcInfo.EndTime.IsZero() {
					tcInfo.EndTime = time.Now()
				}
			}

			for _, tcInfo := range ts.twitchData {
				if !tcInfo.StartTime.IsZero() && time.Since(tcInfo.StartTime) > constants.TwitchStateChangeTime {
					for guild, discordChannels := range tcInfo.DiscordChannels {
						if connected, available := guildStatus[guild]; available && connected {
							for _, discordChannel := range discordChannels {
								if !discordChannel.LiveNotificationSent {
									discordChannel.LiveNotificationSent = true
									go func() {
										if _, err := ds.ChannelMessageSendEmbed(discordChannel.ChannelID, createDiscordEmbedMessage(tcInfo)); err != nil {
											utils.Log.WithError(err).Debug("Error sending message to discord.")
										}
									}()
								}
							}
						}
					}
				} else if !tcInfo.EndTime.IsZero() && time.Since(tcInfo.EndTime) > constants.TwitchStateChangeTime {
					for guild, discordChannels := range tcInfo.DiscordChannels {
						if connected, available := guildStatus[guild]; available && connected {
							for _, discordChannel := range discordChannels {
								if discordChannel.LiveNotificationSent {
									discordChannel.LiveNotificationSent = false
									go func() {
										if _, err := ds.ChannelMessageSend(discordChannel.ChannelID, tcInfo.DisplayName+" is now offline!"); err != nil {
											utils.Log.WithError(err).Debug("Error sending message to discord.")
										}
									}()
								}
							}
						}
					}
				}
			}
		}

		time.Sleep(constants.TwitchQueryInterval)
	}

	delete(activeSessions, ds.State.SessionID)
}

func readGobFromDisk(path string, name string, o *map[string]*twitchChannelInfo) error {
	if file, err := os.Open(path + "/" + name + ".gob"); err != nil {
		return err
	} else {
		return gob.NewDecoder(file).Decode(o)
	}
}

func remove(s []*discordChannel, i int) []*discordChannel {
	s[len(s)-1], s[i] = s[i], s[len(s)-1]
	return s[:len(s)-1]
}

func validateAndRefreshAuthToken(ts *Session) bool {
	// Validate and refresh Twitch authorization token, if token valid
	if isValid, resp, err := ts.client.ValidateToken(ts.client.GetAppAccessToken()); err != nil {
		utils.Log.WithError(err).Error("Failed to validate Twitch authorization token.")
	} else if !isValid {
		ts.isConnected = false
		for !ts.isConnected {
			utils.Log.Debug("Attempting to get new Twitch authentication token.")
			if ts.GetAuthToken() != nil {
				utils.Log.WithError(err).Error("Failed to get new Twitch authorization token.")
				break
			}
		}

		if ts.isConnected {
			utils.Log.Debug("Successfully got new Twitch authentication token.")
			return true
		}
	} else if resp.StatusCode != 200 {
		utils.Log.WithField("StatusCode", resp.StatusCode).Error("HTTP Error returned from twitch.")
	} else {
		return true
	}

	return false
}

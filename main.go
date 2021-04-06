package main

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/ogg"
)

var (
	//go:embed sound/*.opus
	fs    embed.FS
	token string = os.Getenv("DISCORD_TOKEN")
	dg    *discordgo.Session
)

func init() {
	if token == "" {
		log.Fatal("discord token is not set")
	}
}

func main() {
	initConn()
	defer closeConn()
	dg.AddHandler(handler)

	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("yeeeey is now running. press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
	fmt.Println("Exit.")
}

func handler(s *discordgo.Session, m *discordgo.MessageCreate) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("recovered: ", err)
		}
	}()

	if m.Author.Bot || m.Author.ID == s.State.User.ID {
		return
	}

	if strings.HasPrefix(m.Content, "/yey") {
		do("sound/yey.opus", s, m)
	}
	if strings.HasPrefix(m.Content, "/boo") {
		do("sound/boo.opus", s, m)
	}
}

func do(fileName string, s *discordgo.Session, m *discordgo.MessageCreate) {
	b, err := fs.ReadFile(fileName)
	if err != nil {
		log.Printf("error opening %s: %s", fileName, err)
		return
	}

	vs, err := voiceState(dg, m.Author.ID, m.GuildID)
	if err != nil {
		log.Printf("error finding voice state of user %s in guild %s: %s", m.Author.ID, m.GuildID, err)
		return
	}

	if err := joinVC(dg, m.GuildID, vs.ChannelID); err != nil {
		log.Printf("error joining voice channel: %s", err)
		return
	}
	defer func() {
		log.Printf("error leaving from voice channel: %s", err)
		leaveVC(m.GuildID)
	}()

	time.Sleep(100 * time.Microsecond)
	if err := play(m.GuildID, b); err != nil {
		log.Printf("error playing sound: %s", err)
		return
	}
}

func initConn() {
	var err error
	dg, err = discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println("error creating Discord session: ", err)
		return
	}

	if err := dg.Open(); err != nil {
		fmt.Println("error opening Discord session: ", err)
	}
}

func closeConn() {
	if err := dg.Close(); err != nil {
		log.Print(err.Error())
	}
}

func joinVC(s *discordgo.Session, guildID, vcID string) error {
	if conn, ok := dg.VoiceConnections[guildID]; ok {
		if conn.ChannelID == vcID {
			return fmt.Errorf("error connecting to voice channel, bot is already joining target voice channel %s of guild %s", conn.ChannelID, conn.GuildID)
		}
	}
	if _, err := s.ChannelVoiceJoin(guildID, vcID, false, true); err != nil {
		return fmt.Errorf("error joining channel %s of guild %s", vcID, guildID)
	}
	return nil
}

func voiceState(s *discordgo.Session, userID, guildID string) (*discordgo.VoiceState, error) {
	g, err := s.State.Guild(guildID)
	if err != nil {
		return nil, fmt.Errorf("error find guild that the message post to: %s", err)
	}
	for _, vs := range g.VoiceStates {
		if vs.UserID == userID {
			return vs, nil
		}
	}
	return nil, fmt.Errorf("error voice channel to join not found")
}

func voiceConnection(guildID string) (*discordgo.VoiceConnection, bool) {
	conn, ok := dg.VoiceConnections[guildID]
	return conn, ok
}

func play(guildID string, dat []byte) error {
	conn, ok := voiceConnection(guildID)
	if !ok {
		return fmt.Errorf("error getting voice connection on guild %s", guildID)
	}

	oggBuf, err := makeOGGBuffer(dat)
	if err != nil {
		return fmt.Errorf("error createing ogg buffer: %w", err)
	}
	if err := conn.Speaking(true); err != nil {
		log.Printf("error update speaking state to on: %s", err)
	}
	for _, buff := range oggBuf {
		conn.OpusSend <- buff
	}
	if err := conn.Speaking(false); err != nil {
		log.Printf("error update speaking state to off: %s", err)
	}
	return nil
}

func leaveVC(guildID string) error {
	conn, ok := dg.VoiceConnections[guildID]
	// ok should not be false because use state is also checked in textChannelIDs
	if !ok {
		return fmt.Errorf("bot is not joining any of this guild voice channel %s", guildID)
	}

	if err := conn.Disconnect(); err != nil {
		return fmt.Errorf("error disconnect from voice channel %s: %w", conn.ChannelID, err)
	}
	return nil
}

func makeOGGBuffer(in []byte) (output [][]byte, err error) {
	od := ogg.NewDecoder(bytes.NewReader(in))
	pd := ogg.NewPacketDecoder(od)

	// Run through the packet decoder appending the bytes to our output [][]byte
	for {
		packet, _, err := pd.Decode()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("error decode on PacketDecoder: %w", err)
			}
			return output, nil
		}
		output = append(output, packet)
	}
}

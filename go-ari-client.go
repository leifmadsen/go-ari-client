package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/bitly/go-nsq"
	"github.com/bitly/nsq/util"
	"go-ari-library"
)

var (
	config        Config
	totalMessages int = 0
	maxInFlight   int = 200
)

type Config struct {
	LookupdHttpAddress []string `json:"lookupd_http_address"`
	Application        string   `json:"application"` // aka Topic
	Channel            string   `json:"channel"`
	MaxInFlight        string   `json:"max_in_flight"`
	TotalMessages      string   `json:"total_messages"`
}

type MessageHandler struct {
	totalMessages int
	messagesShown int
}

func init() {
	var err error

	// parse the configuration file and get data from it
	configpath := flag.String("config", "./config_client.json", "Path to config file")
	flag.Parse()
	configfile, err := ioutil.ReadFile(*configpath)
	if err != nil {
		log.Fatal(err)
	}

	// read in the configuration file and unmarshal the json, storing it in 'config'
	json.Unmarshal(configfile, &config)
}

func GenHandler() (func(*nsq.Message) error, chan []byte) {
	in := make(chan []byte)
	return func(m *nsq.Message) error {
		in <- m.Body
		return nil
	}, in
}

func PublishCommand(channel string, command *nv.NV_Command, p *nsq.Producer) {
	busMessage, _ := json.Marshal(command)

	fmt.Printf("[DEBUG] Bus Data for %s:\n%s", channel, busMessage)
	p.Publish(channel, []byte(busMessage))
}

func ConsumeEvents(events chan *nv.NV_Event) {
	for event := range events {
		switch event.Type {
		case "StasisStart":
			fmt.Println("Got start message")
		case "ChannelDtmfReceived":
			fmt.Println("Got DTMF")
		case "ChannelHangupRequest":
			fmt.Println("Channel hung up")
		case "StasisEnd":
			fmt.Println("Got end message")
		}
	}
}

func main() {
	fmt.Println("Welcome to the go-ari-client")

	// initial setup
	if config.Application == "" {
		log.Fatal("Missing Application configuration")
	}

	if len(config.LookupdHttpAddress) == 0 {
		log.Fatal("Missing Lookupd HTTP Address configuration")
	}

	if config.Channel == "" {
		rand.Seed(time.Now().UnixNano())
		config.Channel = fmt.Sprintf("tail%06d#ephemeral", rand.Int()%999999)
	}

	if config.MaxInFlight != "" {
		maxInFlight, _ = strconv.Atoi(config.MaxInFlight)
	}

	if config.TotalMessages != "" {
		totalMessages, _ = strconv.Atoi(config.TotalMessages)
	}

	// listen for exit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Don't ask for more messages than we want
	if totalMessages > 0 && totalMessages < maxInFlight {
		maxInFlight = totalMessages
	}

	// connect to nsq and get the json then save to a value
	cfg := nsq.NewConfig()
	cfg.UserAgent = fmt.Sprintf("go_ari_client/%s go-nsq/%s", util.BINARY_VERSION, nsq.VERSION)
	cfg.MaxInFlight = maxInFlight

	// create new consumer and attach to lookupd
	consumer, err := nsq.NewConsumer(config.Application, config.Channel, cfg)
	if err != nil {
		log.Fatal(err)
	}
	err = consumer.ConnectToNSQLookupds(config.LookupdHttpAddress)
	if err != nil {
		log.Fatal(err)
	}

	// generate a handler for processing inbound events
	processEvent, inboundEvents := GenHandler()

	// process events as they come off the bus
	consumer.AddHandler(nsq.HandlerFunc(processEvent))

	// create channel to place parsed events onto
	parsedEvents := make(chan *nv.NV_Event)

	// create consumer that uses the inboundEvents and parses them onto the parsedEvents channel
	nv.InitConsumer(inboundEvents, parsedEvents)

	// start consuming the parsed events
	go ConsumeEvents(parsedEvents)

	// pull the events off the bus
	for {
		select {
		case <-consumer.StopChan:
			return
		case <-sigChan:
			consumer.Stop()
		}
	}
}

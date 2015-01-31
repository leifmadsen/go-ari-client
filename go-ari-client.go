package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"syscall"

	"go-ari-library"
)

var (
	config        Config
)

type Config struct {
	Applications	[]string		`json:"applications"`
	MessageBus		string			`json:"message_bus"`
	BusConfig		interface{}		`json:"bus_config"`
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

func startnvcApp(app string) {
	fmt.Println("Started nvc app")
	application := new(ari.App)
	application.Init(app, nvcHandler)
	select {
	case <- application.Stop:
		return
	}
	
}
// ConsumeEvents pulls events off the channel and passes to the application.
func nvcHandler(a *ari.AppInstance) {
	// this is where you would hand off the information to your application
	for event := range a.Events {
		fmt.Println("got event")
		switch event.Type {
		case "StasisStart":
			var s ari.StasisStart
			json.Unmarshal([]byte(event.ARI_Body), &s)
			a.ChannelsAnswer(s.Channel.Id)
			fmt.Println("Got start message")
		case "ChannelDtmfReceived":
			var c ari.ChannelDtmfReceived
			fmt.Println("Got DTMF")
			json.Unmarshal([]byte(event.ARI_Body), &c)
			fmt.Printf("We got DTMF: %s\n", c.Digit)
			switch c.Digit {
			case "1":
				a.ChannelsPlay(c.Channel.Id, "sound:tt-monkeys", "en")
			case "2":
				a.ChannelsPlay(c.Channel.Id, "sound:tt-weasels")
			case "3":
				a.ChannelsPlay(c.Channel.Id, "sound:demo-congrats")
			case "4":
				err := a.MailboxesUpdate("1234@test", 0, 0)
				if err != nil {
					fmt.Println(err)
				}
			case "5":
				m, err := a.MailboxesGet("1234@test")
				if err != nil {
					fmt.Println(err)
				} else {
					fmt.Printf("Mailbox info is: %v", m)
				}
			}
		case "ChannelHangupRequest":
			fmt.Println("Channel hung up")
		case "StasisEnd":
			fmt.Println("Got end message")
		}
	}
}


// signalCatcher is a function to allows us to stop the application through an
// operating system signal.
func signalCatcher() {
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT)
	sig := <-ch
	log.Printf("Signal received: %v", sig)
	os.Exit(0)
}

func main() {
	fmt.Println("Welcome to the go-ari-client")
	ari.InitBus(config.MessageBus, config.BusConfig)
	
	for _, app := range config.Applications {
		// create consumer that uses the inboundEvents and parses them onto the parsedEvents channel
		
		if app == "nvisible_control" {
			go startnvcApp(app)
		}

	}

	// TODO(leif): make this a go routine
	// pull the events off the bus
	// (brad): this is nsq-specific, needs to move there somehow
	/*
	for {
		select {
		case <-consumer.StopChan:
			return
		case <-sigChan:
			consumer.Stop()
		}
	}
	*/
	go signalCatcher()	// listen for os signal to stop the application
	select{}
}

package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/thoj/go-ircevent"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
)

var config = flag.String("config", "", "configuration file")

type Config struct {
	Irc struct {
		Ssl           bool     `json:"ssl"`
		SslVerifySkip bool     `json:"ssl_verify_skip"`
		Port          string   `json:"port"`
		Nickname      string   `json:"nickname"`
		Channels      []string `json:"channels"`
		Host          string   `json:"host"`
		Password      string   `json:"password"`
	} `json:"irc"`
	Github struct {
		Token string `json:"token"`
		Owner string `json:"owner"`
		Repos string `json:"repos"`
	} `json:"github"`
}

func (c *Config) Load(filename string) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return err
	}

	if c.Irc.Nickname == "" {
		c.Irc.Nickname = "issuebot"
	}

	if c.Irc.Host == "" {
		return errors.New("host is required.")
	}

	if c.Github.Token == "" {
		return errors.New("token is required.")
	}

	if c.Github.Owner == "" {
		return errors.New("owner is required.")
	}

	if c.Github.Repos == "" {
		return errors.New("repos is required.")
	}

	return nil
}

func main() {
	flag.Parse()
	c := &Config{}
	if err := c.Load(*config); err != nil {
		log.Fatal(err)
	}

	ircproj := irc.IRC(c.Irc.Nickname, c.Irc.Nickname)
	ircproj.UseTLS = c.Irc.Ssl
	if c.Irc.SslVerifySkip {
		ircproj.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}
	ircproj.Password = c.Irc.Password

	err := ircproj.Connect(net.JoinHostPort(c.Irc.Host, c.Irc.Port))
	if err != nil {
		log.Fatal(err)
	}

	ircproj.AddCallback("001", func(event *irc.Event) {
		for _, channel := range c.Irc.Channels {
			ircproj.Join(channel)
		}
	})

	r := regexp.MustCompile(`#(\d+)`)
	ircproj.AddCallback("PRIVMSG", func(event *irc.Event) {
		matches := r.FindAllStringSubmatch(event.Message(), -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			u, err := url.Parse(fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%s", c.Github.Owner, c.Github.Repos, match[1]))
			if err != nil {
				log.Println(err)
				continue
			}
			q := u.Query()
			q.Add("access_token", c.Github.Token)
			u.RawQuery = q.Encode()
			resp, err := http.Get(u.String())
			if err != nil {
				log.Println(err)
				continue
			}
			if !(200 <= resp.StatusCode && resp.StatusCode <= 299) {
				log.Println(resp.Status)
				continue
			}
			defer resp.Body.Close()
			m := make(map[string]interface{})
			if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
				log.Println(err)
				continue
			}
			ircproj.Noticef(event.Arguments[0], "#%v %v %v", m["number"].(float64), m["title"].(string), m["html_url"].(string))
		}
	})

	ircproj.Loop()
}

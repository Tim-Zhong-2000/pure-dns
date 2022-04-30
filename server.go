package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/kkyr/fig"
	"github.com/miekg/dns"
)

type server struct {
	Net       string     `fig:"net" default:"udp"`
	Listen    string     `fig:"listen" default:"0.0.0.0:53"`
	Timeout   int        `fig:"timeout" default:"1000"`
	Upstreams []upstream `fig:"upstreams" default:"[]"`
}

func MakeServer() (ok bool, server server) {
	err := fig.Load(&server,
		fig.File("setting.json"),
		fig.Dirs("/etc/pure-dns", "."),
	)
	if err != nil {
		log.Print(err.Error())
		log.Printf("Load config failed!")
		ok = false
		return
	}
	ok = true
	return
}

func (s *server) ListenAndServe() {
	dns.HandleFunc(".", func(w dns.ResponseWriter, req *dns.Msg) {
		_, res := s.Resolve(req)
		w.WriteMsg(res)
	})
	server := &dns.Server{Addr: s.Listen, Net: s.Net}
	log.Printf("Starting DNS server on %s://%s", s.Net, s.Listen)
	err := server.ListenAndServe()
	defer server.Shutdown()
	if err != nil {
		log.Fatalf("Failed to start server: %s\n ", err.Error())
	}
}

func (server *server) Resolve(req *dns.Msg) (ok bool, res *dns.Msg) {
	c := make(chan *dns.Msg)
	defer close(c)
	var lock sync.Mutex
	isClosed := false

	for _, item := range server.Upstreams {
		go func(upstream upstream) {
			if ok, res, rtt := upstream.Resolve(req); ok {
				identRR, _ := dns.NewRR(fmt.Sprintf("%s TXT %s://%s ttl:%s", "dns.provider", upstream.Net, upstream.Address, rtt.String()))
				res.Answer = append(res.Answer, identRR)
				lock.Lock()
				defer lock.Unlock()
				if !isClosed {
					c <- res
					isClosed = true
				}
			}
		}(item)
	}
	select {
	case result := <-c:
		result.SetReply(req)
		return true, result
	case <-time.After(time.Duration(server.Timeout) * time.Millisecond):
		emptyMsg := dns.Msg{}
		emptyMsg.SetReply(req)
		return false, &emptyMsg
	}
}

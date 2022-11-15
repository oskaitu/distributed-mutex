package main

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	beer "github.com/oskaitu/distributed-mutex/grpc"
	"google.golang.org/grpc"
	glog "google.golang.org/grpc/grpclog"
)

var grpcLog glog.LoggerV2

var amount_of_peers = 3

type State int

const (
	Released State = iota
	Wanted
	Held
)

type Peer struct {
	beer.UnimplementedDistributedMutexServer
	id          int32
	state       State
	time        int32
	timeLock    sync.Mutex
	requestTime int32
	drinkWG     sync.WaitGroup
	clients     map[int32]beer.DistributedMutexClient
	ctx         context.Context
}

var mutex = &sync.Mutex{}

func init() {
	grpcLog = glog.NewLoggerV2(os.Stdout, os.Stdout, os.Stdout)
}

func main() {
	// Port
	arg1, _ := strconv.ParseInt(os.Args[1], 10, 32)
	ownPort := int32(arg1) + 5001

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &Peer{
		id:      ownPort,
		state:   Released,
		time:    0,
		clients: make(map[int32]beer.DistributedMutexClient),
		ctx:     ctx,
	}

	// Create listener tcp on port ownPort
	list, err := net.Listen("tcp", fmt.Sprintf(":%v", ownPort))
	if err != nil {
		grpcLog.Errorf("Failed to listen on port: %v", err)
	}
	grpcServer := grpc.NewServer()
	beer.RegisterDistributedMutexServer(grpcServer, p)

	go func() {
		if err := grpcServer.Serve(list); err != nil {
			grpcLog.Errorf("failed to server %v", err)
		}
	}()

	for i := 0; i < amount_of_peers; i++ {
		port := int32(5001) + int32(i)

		if port == ownPort {
			continue
		}

		var conn *grpc.ClientConn
		grpcLog.Infof("Trying to dial: %v\n", port)
		conn, err := grpc.Dial(fmt.Sprintf(":%v", port), grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			grpcLog.Errorf("Could not connect: %s", err)
		}
		defer conn.Close()
		c := beer.NewDistributedMutexClient(conn)
		p.clients[port] = c
	}

	for {
		if p.state == Released {
			sleepTime(randomNumberGenerator(2, 7))
			grpcLog.Infof("Trying to take the critical function: %v : %v\n", p.id, p.time)
			p.requestToDrink()
		}
	}
}

func (p *Peer) Drink(ctx context.Context, req *beer.Request) (*beer.Reply, error) {
	// Lamport time
	if p.time < req.Timestamp {
		p.time = req.Timestamp
	}
	p.time++

	if p.state == Held || (p.state == Wanted && p.requestIsBefore(req)) {
		p.drinkWG.Wait()
	}

	rep := &beer.Reply{Id: p.id, Timestamp: p.time}
	return rep, nil
}

func (p *Peer) requestToDrink() {
	// Updated time
	p.updateClock(p.time)
	p.requestTime = p.time

	// WG
	p.drinkWG.Add(1)

	// Set state
	p.state = Wanted

	// Request
	req := beer.Request{
		Timestamp: p.requestTime,
		Id:        p.id,
	}

	// Wait group
	var wg sync.WaitGroup
	for _, client := range p.clients {
		wg.Add(1)
		go func(c beer.DistributedMutexClient) {
			defer wg.Done()
			reply, _ := c.Drink(p.ctx, &req)
			p.updateClock(reply.Timestamp)
			grpcLog.Infof("Reply from %d At time: %v", reply.Id, reply.Timestamp)
		}(client)
	}

	wg.Wait()
	p.state = Held
	p.drink()
}

func (p *Peer) drink() {
	defer p.releaseBeer()
	grpcLog.Infof("Drinking... At time :%v", p.time)
	sleepTime(randomNumberGenerator(2, 7))
	grpcLog.Infof("Done drinking. At time :%v", p.time)
}

func (p *Peer) releaseBeer() {
	p.state = Released
	p.drinkWG.Done()
}

func (p *Peer) updateClock(time int32) {
	p.timeLock.Lock()
	defer p.timeLock.Unlock()

	if p.time < time {
		p.time = time
	}
	p.time++
}

func (p *Peer) requestIsBefore(req *beer.Request) bool {
	if req.Timestamp < p.time || (req.Timestamp == p.time && req.Id < p.time) {
		return true
	}
	return false
}

func sleepTime(n int) {
	time.Sleep(time.Duration(n) * time.Second)
}

func randomNumberGenerator(min int, max int) int {
	rand.Seed(time.Now().UnixNano())
	n := (rand.Intn(max-min) + min)
	return n
}

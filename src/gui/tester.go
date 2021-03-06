package gui

import (
	"fmt"
	"github.com/skycoin/bbs/src/misc"
	"github.com/skycoin/bbs/src/store"
	"github.com/skycoin/bbs/src/store/typ"
	"github.com/skycoin/cxo/skyobject"
	"github.com/skycoin/skycoin/src/cipher"
	"log"
	"time"
)

// TesterConfig represents configuration for a Tester.
type TesterConfig struct {
	ThreadCount int // Number of threads to use/create for test mode.
	UsersCount  int // Number of master users to use for test mode.
	PostCap     int // Maximum number of posts allowed (negative to disable).
	MinInterval int // Minimum interval between simulated activity (in seconds).
	MaxInterval int // Maximum interval between simulated activity (in seconds).
	Timeout     int // Will stop simulated activity after this time (in seconds, negative to disable).
}

// Tester represents a tester.
// It autonomously creates threads and posts.
type Tester struct {
	config *TesterConfig
	g      *Gateway
	bpk    cipher.PubKey
	tRefs  skyobject.References
	users  []store.UserConfig
	pCap   bool
	pNum   int
	pCount int
	quit   chan struct{}
}

// NewTester creates a new tester.
func NewTester(config *TesterConfig, gateway *Gateway) (*Tester, error) {
	t := &Tester{
		config: config,
		g:      gateway,
		pCap:   config.PostCap >= 0,
		pNum:   1,
		quit:   make(chan struct{}),
	}
	if e := t.setupUsers(); e != nil {
		return nil, e
	}
	if e := t.setupBoard(); e != nil {
		return nil, e
	}
	go t.service()
	return t, nil
}

func (t *Tester) setupUsers() error {
	log.Println("[TESTER] Setting up users...")
	nGoal := t.config.UsersCount
	nNow := len(t.g.Users.Masters.getAll())
	for i := 0; i < nGoal-nNow; i++ {
		t.g.Users.Masters.add(misc.MakeRandomAlias(), misc.MakeTimeStampedRandomID(100).Hex())
	}
	t.users = t.g.Users.Masters.getAll()
	log.Printf("[TESTER] \t- Users: %v", t.users)
	return nil
}

func (t *Tester) setupBoard() error {
	log.Println("[TESTER] Setting up test board...")
	seed := misc.MakeTimeStampedRandomID(100).Hex()
	pk, _ := cipher.GenerateDeterministicKeyPair([]byte(seed))
	if e := t.g.Tests.addFilledBoard(seed, t.config.ThreadCount, 1, 1); e != nil {
		return e
	}
	t.bpk = pk
	log.Printf("[TESTER] \t- Board: '%s'", t.bpk.Hex())
	threads := t.g.Threads.getAll(t.bpk)
	log.Printf("[TESTER] \t- Threads(%d):", len(threads))
	t.tRefs = make(skyobject.References, len(threads))
	for i, thread := range threads {
		t.tRefs[i] = thread.GetRef()
		log.Printf("[TESTER] \t\t- [%d] '%s'", i, t.tRefs[i].String())
	}
	return nil
}

func (t *Tester) service() {
	if t.config.Timeout >= 0 {
		log.Printf("[TESTER] Test mode timeout set as %ds.", t.config.Timeout)
		go func() {
			timer := time.NewTimer(time.Duration(t.config.Timeout) * time.Second)
			for {
				select {
				case <-t.quit:
					return
				case <-timer.C:
					log.Println("[TESTER] Test mode timeout done.")
					t.Close()
					return
				}
			}
		}()
	}
	for {
		interval := t.getInterval()
		log.Printf("[TESTER] (PAUSE: %ds)", interval/time.Second)
		time.Sleep(interval)

		select {
		case <-t.quit:
			log.Println("[TESTER] Closing...")
			return
		default:
			break
		}

		choice, _ := misc.MakeIntBetween(0, 10)
		t.actionChangeUser()
		switch choice {
		case 0, 1, 2:
			log.Println("[TESTER] <<< Action: New Post >>>")
			t.actionNewPost()
		case 3:
			log.Println("[TESTER] <<< Action: Remove Post >>>")
			t.actionDeletePost()
		case 4, 5, 6, 7:
			log.Println("[TESTER] <<< Action: Vote Post >>>")
			t.actionVotePost()
		case 8, 9, 10:
			log.Println("[TESTER] <<< Action: Vote Thread >>>")
			t.actionVoteThread()
		}
	}
}

func (t *Tester) Close() {
	timer := time.NewTimer(time.Duration(t.config.MaxInterval) * time.Second)
	select {
	case t.quit <- struct{}{}:
		log.Println("[TESTER] Sent quit signal.")
	case <-timer.C:
	}
}

func (t *Tester) getInterval() time.Duration {
	i, e := misc.MakeIntBetween(
		t.config.MinInterval,
		t.config.MaxInterval,
	)
	if e != nil {
		log.Printf("[TESTER] \t- (ERROR) '%s'.", e)
		return time.Second
	}
	return time.Duration(i) * time.Second
}

func (t *Tester) getRandomThreadRef() skyobject.Reference {
	i, e := misc.MakeIntBetween(0, len(t.tRefs)-1)
	if e != nil {
		log.Printf("[TESTER] \t- (ERROR) '%s'.", e)
		return skyobject.Reference{}
	}
	return t.tRefs[i]
}

func (t *Tester) getPostNum() int {
	defer func() { t.pNum += 1 }()
	return t.pNum
}

func (t *Tester) getPostCount() int {
	return t.pCount
}

func (t *Tester) getRandomPostRef(tRef skyobject.Reference) (skyobject.Reference, bool) {
	posts, e := t.g.Posts.get(t.bpk, tRef)
	if e != nil {
		log.Printf("[TESTER] \t- (ERROR) '%s'.", e)
		return skyobject.Reference{}, false
	}
	if len(posts) == 0 {
		return skyobject.Reference{}, false
	}
	i, e := misc.MakeIntBetween(0, len(posts)-1)
	if e != nil {
		log.Printf("[TESTER] \t- (ERROR) '%s'.", e)
		return skyobject.Reference{}, false
	}
	ref, e := misc.GetReference(posts[i].Ref)
	if e != nil {
		log.Printf("[TESTER] \t- (ERROR) '%s'.", e)
		return skyobject.Reference{}, false
	}
	return ref, true
}

func (t *Tester) actionChangeUser() {
	i, e := misc.MakeIntBetween(0, len(t.users)-1)
	if e != nil {
		log.Printf("[TESTER] \t- (ERROR) '%s'.", e)
		return
	}
	if e := t.g.Users.Masters.Current.set(t.users[i].GetPK()); e != nil {
		log.Printf("[TESTER] \t- (ERROR) '%s'.", e)
		return
	}
}

func (t *Tester) actionNewPost() {
	if t.pCap && t.pCount >= t.config.PostCap {
		log.Println("[TESTER] \t- Post cap reached. Continuing...")
		return
	}
	user := t.g.Users.Masters.Current.get()
	log.Printf("[TESTER] \t- User: %s '%s'", user.Alias, user.PubKey)
	tRef := t.getRandomThreadRef()
	log.Printf("[TESTER] \t- Thread: '%s'", tRef.String())
	post := &typ.Post{
		Title: fmt.Sprintf("Test Post %d", t.getPostNum()),
		Body:  fmt.Sprintf("This is a test post by test user %s.", user.Alias),
	}
	log.Printf("[TESTER] \t- Post: {Title: '%s', Body: '%s'}", post.Title, post.Body)
	if e := post.Sign(user.GetPK(), user.GetSK()); e != nil {
		log.Printf("[TESTER] \t- (ERROR) '%s'.", e)
		return
	}
	if e := t.g.Posts.add(t.bpk, tRef, post); e != nil {
		log.Printf("[TESTER] \t- (ERROR) '%s'.", e)
	}
}

func (t *Tester) actionDeletePost() {
	tRef := t.getRandomThreadRef()
	log.Printf("[TESTER] \t- Thread: '%s'", tRef.String())
	pRef, has := t.getRandomPostRef(tRef)
	if !has {
		log.Printf("[TESTER] \t- No posts here. Continuing...")
		return
	} else {
		log.Printf("[TESTER] \t- Post: '%s'", pRef.String())
	}
	if e := t.g.Posts.remove(t.bpk, tRef, pRef); e != nil {
		log.Printf("[TESTER] \t- (ERROR) '%s'.", e)
	}
}

func (t *Tester) actionVotePost() {
	user := t.g.Users.Masters.Current.get()
	log.Printf("[TESTER] \t- User: %s '%s'", user.Alias, user.PubKey)
	pRef, has := t.getRandomPostRef(t.getRandomThreadRef())
	if !has {
		log.Printf("[TESTER] \t- No posts here. Continuing...")
		return
	} else {
		log.Printf("[TESTER] \t- Post: '%s'", pRef.String())
	}
	vMode, e := misc.MakeIntBetween(-1, +1)
	log.Printf("[TESTER] \t- Mode: %d", vMode)
	if e != nil {
		log.Printf("[TESTER] \t- (ERROR) '%s'.", e)
		return
	}
	vote := &typ.Vote{Mode: int8(vMode)}
	if e := vote.Sign(user.GetPK(), user.GetSK()); e != nil {
		log.Printf("[TESTER] \t- (ERROR) '%s'.", e)
		return
	}
	if e := t.g.Posts.Votes.add(t.bpk, pRef, vote); e != nil {
		log.Printf("[TESTER] \t- (ERROR) '%s'.", e)
		return
	}
}

func (t *Tester) actionVoteThread() {
	user := t.g.Users.Masters.Current.get()
	log.Printf("[TESTER] \t- User: %s '%s'", user.Alias, user.PubKey)
	tRef := t.getRandomThreadRef()
	log.Printf("[TESTER] \t- Thread: '%s'", tRef.String())
	vMode, e := misc.MakeIntBetween(-1, +1)
	log.Printf("[TESTER] \t- Mode: %d", vMode)
	if e != nil {
		log.Printf("[TESTER] \t- (ERROR) '%s'.", e)
		return
	}
	vote := &typ.Vote{Mode: int8(vMode)}
	if e := vote.Sign(user.GetPK(), user.GetSK()); e != nil {
		log.Printf("[TESTER] \t- (ERROR) '%s'.", e)
		return
	}
	if e := t.g.Threads.Votes.add(t.bpk, tRef, vote); e != nil {
		log.Printf("[TESTER] !!! Error: %s !!!", e.Error())
	}
}

//func (t *Tester) actionVoteUser() {
//	i, e := misc.MakeIntBetween(0, len(t.users)-1)
//	if e != nil {
//
//	}
//	t.users[i].GetPK()
//}

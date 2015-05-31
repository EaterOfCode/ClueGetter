// GlueGetter - Does things with mail
//
// Copyright 2015 Dolf Schimmel, Freeaqingme.
//
// This Source Code Form is subject to the terms of the two-clause BSD license.
// For its contents, please refer to the LICENSE file.
//
package cluegetter

// Todo: What if multiple messages are sent over single connection?
// Todo: Clean up sessions

import (
	"encoding/json"
	"fmt"
	m "github.com/Freeaqingme/gomilter"
	"github.com/nu7hatch/gouuid"
	"sync"
	"time"
	"strings"
)

type milter struct {
	m.MilterRaw
}

type milterDataIndex struct {
	sessions map[string]*milterSession
	mu       sync.RWMutex
}

func (di *milterDataIndex) getNewSession() *milterSession {
	u, err := uuid.NewV4()
	if err != nil {
		panic(fmt.Sprintf("Could not generate UUID. Lack of entropy? Error: %s"))
	}

	di.mu.Lock()
	defer di.mu.Unlock()

	data := &milterSession{id: u.String(), timeStart: time.Now()}
	di.sessions[u.String()] = data
	return data
}

var MilterDataIndex milterDataIndex

func milterStart() {
	MilterDataIndex = milterDataIndex{sessions: make(map[string]*milterSession)}

	//	m.LoggerPrintln = milterLog
	//	m.LoggerPrintf = Log.Debug

	StatsCounters["MilterCallbackConnect"] = &StatsCounter{}
	StatsCounters["MilterCallbackHelo"] = &StatsCounter{}
	StatsCounters["MilterCallbackEnvFrom"] = &StatsCounter{}
	StatsCounters["MilterCallbackEnvRcpt"] = &StatsCounter{}
	StatsCounters["MilterCallbackHeader"] = &StatsCounter{}
	StatsCounters["MilterCallbackEnvFromErrors"] = &StatsCounter{}

	milter := new(milter)
	milter.FilterName = "GlueGetter"
	milter.Debug = true
	milter.Flags = m.ADDHDRS | m.ADDRCPT | m.CHGFROM | m.CHGBODY
	milter.Socket = "inet:10033@127.0.0.1" // Todo: Should be configurable

	go func() {
		if m.Run(milter) == -1 {
			// Todo: May just want to retry?
			Log.Fatal("libmilter returned an error.")
		}
	}()

}

func (milter *milter) Connect(ctx uintptr, hostname, ip string) (sfsistat int8) {
	d := MilterDataIndex.getNewSession()
	d.Hostname = hostname
	d.Ip = ip
	m.SetPriv(ctx, d.getId())

	StatsCounters["MilterCallbackConnect"].increase(1)
	Log.Debug("%s Milter.Connect called: ip = %s, hostname = %s", d.getId(), ip, hostname)

	return m.Continue
}

func (milter *milter) Helo(ctx uintptr, helo string) (sfsistat int8) {
	d := milterGetSession(ctx, true)
	d.Helo = helo
	d.CertIssuer = m.GetSymVal(ctx, "{cert_issuer}")
	d.CertSubject = m.GetSymVal(ctx, "{cert_subject}")
	d.CipherBits = m.GetSymVal(ctx, "{cipher_bits}")
	d.Cipher = m.GetSymVal(ctx, "{cipher}")
	d.TlsVersion = m.GetSymVal(ctx, "{tls_version}")

	StatsCounters["MilterCallbackHelo"].increase(1)
	Log.Debug("%s Milter.Helo called: helo = %s", d.getId(), helo)

	return
}

func (milter *milter) EnvFrom(ctx uintptr, from []string) (sfsistat int8) {
	d := milterGetSession(ctx, true)
	msg := d.getNewMessage()

	StatsCounters["MilterCallbackEnvFrom"].increase(1)
	Log.Debug("%s Milter.EnvFrom called: from = %s", d.getId(), from[0])

	if len(from) != 1 {
		StatsCounters["MilterCallbackEnvFromErrors"].increase(1)
		Log.Critical("%s Milter.EnvFrom callback received %d elements: %s", d.getId(), len(from), fmt.Sprint(from))
	}
	msg.From = from[0]
	return
}

func (milter *milter) EnvRcpt(ctx uintptr, rcpt []string) (sfsistat int8) {
	d := milterGetSession(ctx, true)
	msg := d.getLastMessage()
	msg.Rcpt = append(msg.Rcpt, rcpt[0])

	StatsCounters["MilterCallbackEnvRcpt"].increase(1)
	Log.Debug("%s Milter.EnvRcpt called: rcpt = %s", d.getId(), fmt.Sprint(rcpt))
	return
}

func (milter *milter) Header(ctx uintptr, headerf, headerv string) (sfsistat int8) {
	d := milterGetSession(ctx, true)
	msg := d.getLastMessage()
	msg.Header = append(msg.Header, &milterMessageHeader{headerf, headerv})

	StatsCounters["MilterCallbackHeader"].increase(1)
	Log.Debug("%s Milter.Header called: header %s = %s", d.getId(), headerf, headerv)
	return
}

func (milter *milter) Eoh(ctx uintptr) (sfsistat int8) {
	d := milterGetSession(ctx, true)
	d.SaslSender = m.GetSymVal(ctx, "{auth_author}")
	d.SaslMethod = m.GetSymVal(ctx, "{auth_type}")
	d.SaslUsername = m.GetSymVal(ctx, "{auth_authen}")
	msg := d.getLastMessage()
	msg.QueueId = m.GetSymVal(ctx, "i")

	Log.Debug("%s milter.Eoh was called", d.getId())
	return
}

func (milter *milter) Body(ctx uintptr, body []byte) (sfsistat int8) {
	bodyStr := string(body)

	d := milterGetSession(ctx, true)
	msg := d.getLastMessage()
	msg.Body = append(msg.Body, bodyStr)

	Log.Debug("%s milter.Body was called. Length of body: %d", d.getId(), len(bodyStr))
	return
}

func (milter *milter) Eom(ctx uintptr) (sfsistat int8) {
	d := milterGetSession(ctx, true)
	Log.Debug("%s milter.Eom was called", d.getId())

	//	fmt.Println(m.SetReply(ctx, "521", "5.7.1", "we dont like you"))
	//	return m.Reject
//	jsonStr, _ := json.Marshal(d)
//	fmt.Println(string(jsonStr))
//	jsonStr, _ = json.Marshal(d.getLastMessage())
//	fmt.Println(string(jsonStr))
//	fmt.Println(strings.Join(d.getLastMessage().Body, ""))
	return
}

func (milter *milter) Abort(ctx uintptr) (sfsistat int8) {
	_ = milterGetSession(ctx, false)
	Log.Debug("milter.Abort was called")
	return
}

func (milter *milter) Close(ctx uintptr) (sfsistat int8) {
	_ = milterGetSession(ctx, false)
	Log.Debug("milter.Close was called")
	return
}

func milterLog(i ...interface{}) {
	Log.Debug(fmt.Sprintf("%s", i[:1]), i[1:]...)
}

func milterGetSession(ctx uintptr, keep bool) *milterSession {
	var u string
	m.GetPriv(ctx, &u)
	if keep {
		m.SetPriv(ctx, u)
	}

	return MilterDataIndex.sessions[u]
}

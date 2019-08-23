package dial

import (
	lg "github.com/ipfs/go-log"
	"github.com/whyrusleeping/go-logging"
)

var log = lg.Logger("swarm/dialer")

var (
	_innerlog = logging.MustGetLogger("swarm/dial")

	// plussing the prefix onto the message is not too bad if we only take the hit iff the log level is
	// enabled; this happens on the stack, hence 0 allocs, and it's very fast. Alternatives include typing
	// the message and providing constructor functions that inject the prefix.
	reqPrefix = "peer %s: "
	jobPrefix = "peer %s, addr %s: "
)

func (r *Request) Debugf(msg string, args ...interface{}) {
	if _innerlog.IsEnabledFor(logging.DEBUG) {
		args = append([]interface{}{r.id}, args...)
		log.Debugf(reqPrefix+msg, args...)
	}
}

func (r *Request) Infof(msg string, args ...interface{}) {
	if _innerlog.IsEnabledFor(logging.INFO) {
		args = append([]interface{}{r.id}, args...)
		log.Infof(reqPrefix+msg, args...)
	}
}

func (r *Request) Warningf(msg string, args ...interface{}) {
	if _innerlog.IsEnabledFor(logging.WARNING) {
		args = append([]interface{}{r.id}, args...)
		log.Warningf(reqPrefix+msg, args...)
	}
}

func (r *Request) Errorf(msg string, args ...interface{}) {
	if _innerlog.IsEnabledFor(logging.ERROR) {
		args = append([]interface{}{r.id}, args...)
		log.Errorf(reqPrefix+msg, args...)
	}
}

func (j *Job) Debugf(msg string, args ...interface{}) {
	if _innerlog.IsEnabledFor(logging.DEBUG) {
		args = append([]interface{}{j.req.id, j.addr}, args...)
		log.Debugf(jobPrefix+msg, args...)
	}
}

func (j *Job) Infof(msg string, args ...interface{}) {
	if _innerlog.IsEnabledFor(logging.INFO) {
		args = append([]interface{}{j.req.id, j.addr}, args...)
		log.Infof(jobPrefix+msg, args...)
	}
}

func (j *Job) Warningf(msg string, args ...interface{}) {
	if _innerlog.IsEnabledFor(logging.WARNING) {
		args = append([]interface{}{j.req.id, j.addr}, args...)
		log.Warningf(jobPrefix+msg, args...)
	}
}

func (j *Job) Errorf(msg string, args ...interface{}) {
	if _innerlog.IsEnabledFor(logging.ERROR) {
		args = append([]interface{}{j.req.id, j.addr}, args...)
		log.Errorf(jobPrefix+msg, args...)
	}
}

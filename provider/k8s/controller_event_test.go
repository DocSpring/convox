package k8s

import (
	"testing"
	"time"

	"github.com/convox/convox/pkg/mock"
	"github.com/stretchr/testify/require"
	ac "k8s.io/api/core/v1"
	am "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type loggedLine struct {
	name, stream, message string
	ts                    time.Time
}

type eventLogEngine struct {
	mock.TestEngine
	lines []loggedLine
}

func (e *eventLogEngine) Log(name, stream string, ts time.Time, message string) error {
	e.lines = append(e.lines, loggedLine{name: name, stream: stream, ts: ts, message: message})
	return nil
}

func newEventLogController(start time.Time) (*EventController, *eventLogEngine) {
	engine := &eventLogEngine{}
	return &EventController{Provider: &Provider{Engine: engine}, start: am.NewTime(start)}, engine
}

func nodeEvent(count int32, last time.Time) *ac.Event {
	return &ac.Event{
		ObjectMeta:     am.ObjectMeta{Namespace: "default", Name: "ip-10-0-0-1.1"},
		InvolvedObject: ac.ObjectReference{Kind: "Node", Name: "ip-10-0-0-1"},
		Type:           "Warning",
		Reason:         "KubeletHasDiskPressure",
		Message:        "Node has disk pressure",
		Count:          count,
		LastTimestamp:  am.NewTime(last),
		Source:         ac.EventSource{Component: "kubelet"},
	}
}

func TestEventControllerLogsEveryKindToSystem(t *testing.T) {
	start := time.Now().Add(-time.Minute)
	c, engine := newEventLogController(start)

	require.NoError(t, c.Add(nodeEvent(1, time.Now())))

	require.Len(t, engine.lines, 1)
	require.Equal(t, "system", engine.lines[0].name)
	require.Equal(t, "k8s-events/default", engine.lines[0].stream)
	require.Equal(t, `type=Warning reason=KubeletHasDiskPressure kind=Node namespace= name=ip-10-0-0-1 count=1 source=kubelet message="Node has disk pressure"`, engine.lines[0].message)
}

func TestEventControllerSkipsEventsFromBeforeStart(t *testing.T) {
	c, engine := newEventLogController(time.Now())

	require.NoError(t, c.Add(nodeEvent(3, time.Now().Add(-time.Hour))))

	require.Empty(t, engine.lines)
}

func TestEventControllerLogsRecurrences(t *testing.T) {
	c, engine := newEventLogController(time.Now().Add(-time.Minute))

	prev := nodeEvent(4, time.Now())
	cur := nodeEvent(5, time.Now())

	require.NoError(t, c.Update(prev, cur))
	require.Len(t, engine.lines, 1)
	require.Contains(t, engine.lines[0].message, "count=5")

	// A resync delivers the same event again: nothing new happened
	require.NoError(t, c.Update(cur, cur))
	require.Len(t, engine.lines, 1)
}

func TestEventControllerEventsAPIEvents(t *testing.T) {
	observed := time.Now().UTC().Truncate(time.Microsecond)
	c, engine := newEventLogController(time.Now().Add(-time.Minute))

	// The scheduler writes through events.k8s.io: no lastTimestamp or count, a series instead
	prev := &ac.Event{
		ObjectMeta:          am.ObjectMeta{Namespace: "app", Name: "web-1.1"},
		InvolvedObject:      ac.ObjectReference{Kind: "Pod", Namespace: "app", Name: "web-1"},
		Type:                "Warning",
		Reason:              "FailedScheduling",
		Message:             "0/3 nodes are available",
		EventTime:           am.NewMicroTime(observed.Add(-30 * time.Second)),
		ReportingController: "default-scheduler",
		Series:              &ac.EventSeries{Count: 2, LastObservedTime: am.NewMicroTime(observed.Add(-10 * time.Second))},
	}
	cur := prev.DeepCopy()
	cur.Series = &ac.EventSeries{Count: 3, LastObservedTime: am.NewMicroTime(observed)}

	require.NoError(t, c.Update(prev, cur))

	require.Len(t, engine.lines, 1)
	require.Equal(t, "k8s-events/app", engine.lines[0].stream)
	require.True(t, observed.Equal(engine.lines[0].ts), "logged at %s, want %s", engine.lines[0].ts, observed)
	require.Contains(t, engine.lines[0].message, "reason=FailedScheduling kind=Pod namespace=app name=web-1 count=3 source=default-scheduler")
}

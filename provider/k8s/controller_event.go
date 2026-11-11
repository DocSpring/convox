package k8s

import (
	"fmt"
	"strings"
	"time"

	"github.com/convox/convox/pkg/kctl"
	"github.com/pkg/errors"
	ac "k8s.io/api/core/v1"
	am "k8s.io/apimachinery/pkg/apis/meta/v1"
	ic "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type EventController struct {
	Controller *kctl.Controller
	Provider   *Provider

	start am.Time
}

func NewEventController(p *Provider) (*EventController, error) {
	pc := &EventController{
		Provider: p,
		start:    am.NewTime(time.Now().UTC()),
	}

	c, err := kctl.NewController(p.Namespace, "convox-k8s-event", pc)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	pc.Controller = c

	return pc, nil
}

func (c *EventController) Client() kubernetes.Interface {
	return c.Provider.Cluster
}

func (c *EventController) Informer() cache.SharedInformer {

	return ic.NewFilteredEventInformer(c.Provider.Cluster, ac.NamespaceAll, 0, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, c.ListOptions)
}

func (c *EventController) ListOptions(opts *am.ListOptions) {}

func (c *EventController) Run() {
	ch := make(chan error)

	go c.Controller.Run(ch)

	for err := range ch {
		fmt.Printf("err = %+v\n", err)
	}
}

func (c *EventController) Start() error {
	return nil
}

func (c *EventController) Stop() error {
	return nil
}

func (c *EventController) Add(obj interface{}) error {
	e, err := assertEvent(obj)
	if err != nil {
		return errors.WithStack(err)
	}

	if e.InvolvedObject.Kind == "Node" && e.Reason == "NodeNotSchedulable" && e.LastTimestamp.Add(5*time.Minute).After(time.Now()) {
		c.Provider.nc.Add(e.InvolvedObject.Name)
	}

	ts := eventTime(e)

	if ts.Before(c.start.Time) {
		return nil
	}

	if err := c.logClusterEvent(e, ts); err != nil {
		return errors.WithStack(err)
	}

	o := e.InvolvedObject

	kind := fmt.Sprintf("%s/%s", e.InvolvedObject.APIVersion, e.InvolvedObject.Kind)

	switch kind {
	case "apps/v1/Deployment":
		app, err := c.Provider.NamespaceApp(o.Namespace)
		if err != nil {
			if strings.Contains(err.Error(), "could not determine app for namespace") {
				return nil
			}
			return errors.WithStack(err)
		}

		if err := c.Provider.systemLog(c.Provider.namespaceTid(o.Namespace, app), app, o.Name, ts, e.Message); err != nil {
			return errors.WithStack(err)
		}
	case "apps/v1/ReplicaSet":
		app, err := c.Provider.NamespaceApp(o.Namespace)
		if err != nil {
			if strings.Contains(err.Error(), "could not determine app for namespace") {
				return nil
			}
			return errors.WithStack(err)
		}

		if err := c.Provider.systemLog(c.Provider.namespaceTid(o.Namespace, app), app, o.Name, ts, e.Message); err != nil {
			return errors.WithStack(err)
		}
	case "atom.convox.com/v1/Atom":
		app, err := c.Provider.NamespaceApp(o.Namespace)
		if err != nil {
			if strings.Contains(err.Error(), "could not determine app for namespace") {
				return nil
			}
			return errors.WithStack(err)
		}

		if err := c.Provider.systemLog(c.Provider.namespaceTid(o.Namespace, app), app, fmt.Sprintf("atom/%s", strings.ReplaceAll(e.InvolvedObject.Name, ".", "/")), ts, fmt.Sprintf("%s: %s", e.Reason, e.Message)); err != nil {
			return errors.WithStack(err)
		}
	case "autoscaling/v2beta2/HorizontalPodAutoscaler":
		app, err := c.Provider.NamespaceApp(o.Namespace)
		if err != nil {
			if strings.Contains(err.Error(), "could not determine app for namespace") {
				return nil
			}
			return errors.WithStack(err)
		}

		if err := c.Provider.systemLog(c.Provider.namespaceTid(o.Namespace, app), app, o.Name, ts, e.Message); err != nil {
			return errors.WithStack(err)
		}
	case "autoscaling/v2/HorizontalPodAutoscaler":
		app, err := c.Provider.NamespaceApp(o.Namespace)
		if err != nil {
			if strings.Contains(err.Error(), "could not determine app for namespace") {
				return nil
			}
			return errors.WithStack(err)
		}

		if err := c.Provider.systemLog(c.Provider.namespaceTid(o.Namespace, app), app, o.Name, ts, e.Message); err != nil {
			return errors.WithStack(err)
		}
	case "v1/ConfigMap":
	case "v1/Pod":
		switch e.Reason {
		case "Killing":
		default:
			app, err := c.Provider.NamespaceApp(o.Namespace)
			if err != nil {
				if strings.Contains(err.Error(), "could not determine app for namespace") {
					return nil
				}
				return errors.WithStack(err)
			}

			if err := c.Provider.systemLog(c.Provider.namespaceTid(o.Namespace, app), app, o.Name, ts, e.Message); err != nil {
				return errors.WithStack(err)
			}
		}
	}

	return nil
}

func (c *EventController) Delete(obj interface{}) error {
	return nil
}

// Update logs an event again when it recurs. Kubernetes doesn't create a new Event for a repeat (a readiness probe
// that keeps failing, a pod that keeps crashing): it raises the existing one's count. Re-lists and resyncs deliver
// updates with the same count, which are skipped.
func (c *EventController) Update(prev, cur interface{}) error {
	pe, err := assertEvent(prev)
	if err != nil {
		return errors.WithStack(err)
	}

	ce, err := assertEvent(cur)
	if err != nil {
		return errors.WithStack(err)
	}

	if eventCount(ce) <= eventCount(pe) {
		return nil
	}

	ts := eventTime(ce)

	if ts.Before(c.start.Time) {
		return nil
	}

	return errors.WithStack(c.logClusterEvent(ce, ts))
}

// logClusterEvent writes one line per event, and per recurrence, for every kind of object in every namespace, to the
// rack's system log (stream k8s-events/<namespace>). Kubernetes keeps events for an hour, and the per-app copies above
// cover only some kinds in app namespaces and only an event's first occurrence.
func (c *EventController) logClusterEvent(e *ac.Event, ts time.Time) error {
	o := e.InvolvedObject

	ns := o.Namespace
	if ns == "" {
		ns = e.Namespace
	}

	line := fmt.Sprintf("type=%s reason=%s kind=%s namespace=%s name=%s count=%d source=%s message=%q",
		e.Type, e.Reason, o.Kind, o.Namespace, o.Name, eventCount(e), eventSource(e), e.Message)

	return errors.WithStack(c.Provider.Engine.Log("system", fmt.Sprintf("k8s-events/%s", ns), ts, line))
}

// eventTime is when the event last happened. Events written through the events.k8s.io API (the scheduler's
// FailedScheduling, for example) leave lastTimestamp empty and record eventTime and series instead.
func eventTime(e *ac.Event) time.Time {
	switch {
	case !e.LastTimestamp.IsZero():
		return e.LastTimestamp.Time
	case e.Series != nil && !e.Series.LastObservedTime.IsZero():
		return e.Series.LastObservedTime.Time
	case !e.EventTime.IsZero():
		return e.EventTime.Time
	default:
		return e.CreationTimestamp.Time
	}
}

// eventCount is how many times the event has happened: count, or series.count for events.k8s.io events.
func eventCount(e *ac.Event) int32 {
	n := e.Count

	if e.Series != nil && e.Series.Count > n {
		n = e.Series.Count
	}

	if n < 1 {
		n = 1
	}

	return n
}

func eventSource(e *ac.Event) string {
	if e.ReportingController != "" {
		return e.ReportingController
	}

	return e.Source.Component
}

func assertEvent(v interface{}) (*ac.Event, error) {
	e, ok := v.(*ac.Event)
	if !ok {
		return nil, errors.WithStack(fmt.Errorf("could not assert deployment for type: %T", v))
	}

	return e, nil
}

package fsm

import (
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go/service/swf"
	. "github.com/sclasen/swfsm/log"
	. "github.com/sclasen/swfsm/sugar"
)

//ComposedDecider can be used to build a decider out of a number of sub Deciders
//the sub deciders should return Pass when they dont wish to handle an event.
type ComposedDecider struct {
	deciders []Decider
}

//NewComposedDecider builds a Composed Decider from a list of sub Deciders.
//You can compose your fiinal composable decider from other composable deciders,
//but you should make sure that the final decider includes a 'catch-all' decider in last place
//you can use DefaultDecider() or your own.
func NewComposedDecider(deciders ...Decider) Decider {
	c := ComposedDecider{
		deciders: deciders,
	}
	return c.Decide
}

//Decide is the the Decider func for a ComposedDecider
func (c *ComposedDecider) Decide(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
	state := ctx.State
	decisions := ctx.EmptyDecisions()
	for _, d := range c.deciders {
		outcome := d(ctx, h, data)
		// contribute the outcome's decisions and data
		decisions = append(decisions, outcome.Decisions...)
		data = outcome.Data
		if outcome.State == "" {
			continue
		}
		state = outcome.State

		return Outcome{
			Data:      data,
			State:     state,
			Decisions: decisions,
		}
	}
	return Outcome{
		Data:      data,
		State:     "",
		Decisions: decisions,
	}
}

func logf(ctx *FSMContext, format string, data ...interface{}) {
	format = fmt.Sprintf("workflow=%s workflow-id=%s state=%s ", LS(ctx.WorkflowType.Name), LS(ctx.WorkflowId), ctx.State) + format
	Log.Printf(format, data...)
}

//DefaultDecider is a 'catch-all' decider that simply logs the unhandled decision.
//You should place this or one like it as the last decider in your top level ComposableDecider.
func DefaultDecider() Decider {
	return func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
		Log.Printf("at=unhandled-event event=%s state=%s default=stay decisions=0", LS(h.EventType), ctx.State)
		return ctx.Stay(data, ctx.EmptyDecisions())
	}
}

//DecisionFunc is a building block for composable deciders that returns a decision.
type DecisionFunc func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) *swf.Decision

//MultiDecisionFunc is a building block for composable deciders that returns a [] of decision.
type MultiDecisionFunc func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) []*swf.Decision

//StateFunc is a building block for composable deciders mutates the FSM stateData.
type StateFunc func(ctx *FSMContext, h *swf.HistoryEvent, data interface{})

//PredicateFunc is a building block for composable deciders, a predicate based on the FSM stateData.
type PredicateFunc func(data interface{}) bool

//Typed allows you to create Typed building blocks for composable deciders.
//the type checking here is done on constriction at runtime, so be sure to have a unit test that constructs your funcs.
func Typed(typed interface{}) *TypedFuncs {
	return &TypedFuncs{typed}
}

//TypedFuncs lets you construct building block for composable deciders, that have arguments that are checked
//against the type of your FSM stateData.
type TypedFuncs struct {
	typed interface{}
}

func (t *TypedFuncs) typeArg() string {
	return reflect.TypeOf(t.typed).String()
}

//Decider builds a Decider from your typed Decider that verifies the right typing at construction time.
func (t *TypedFuncs) Decider(decider interface{}) Decider {
	typeCheck(decider, []string{"*fsm.FSMContext", "*swf.HistoryEvent", t.typeArg()}, []string{"fsm.Outcome"})
	return marshalledFunc{reflect.ValueOf(decider)}.decider
}

//DecisionFunc builds a DecisionFunc from your typed DecisionFunc that verifies the right typing at construction time.
func (t *TypedFuncs) DecisionFunc(decisionFunc interface{}) DecisionFunc {
	typeCheck(decisionFunc, []string{"*fsm.FSMContext", "*swf.HistoryEvent", t.typeArg()}, []string{"*swf.Decision"})
	return marshalledFunc{reflect.ValueOf(decisionFunc)}.decisionFunc
}

//MultiDecisionFunc builds a MultiDecisionFunc from your typed MultiDecisionFunc that verifies the right typing at construction time.
func (t *TypedFuncs) MultiDecisionFunc(decisionFunc interface{}) MultiDecisionFunc {
	typeCheck(decisionFunc, []string{"*fsm.FSMContext", "*swf.HistoryEvent", t.typeArg()}, []string{"[]*swf.Decision"})
	return marshalledFunc{reflect.ValueOf(decisionFunc)}.multiDecisionFunc
}

//StateFunc builds a StateFunc from your typed StateFunc that verifies the right typing at construction time.
func (t *TypedFuncs) StateFunc(stateFunc interface{}) StateFunc {
	typeCheck(stateFunc, []string{"*fsm.FSMContext", "*swf.HistoryEvent", t.typeArg()}, []string{})
	return marshalledFunc{reflect.ValueOf(stateFunc)}.stateFunc
}

//PredicateFunc builds a PredicateFunc from your typed PredicateFunc that verifies the right typing at construction time.
func (t *TypedFuncs) PredicateFunc(stateFunc interface{}) PredicateFunc {
	typeCheck(stateFunc, []string{t.typeArg()}, []string{"bool"})
	return marshalledFunc{reflect.ValueOf(stateFunc)}.predicateFunc
}

type marshalledFunc struct {
	v reflect.Value
}

func (m marshalledFunc) decider(f *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
	ret := m.v.Call([]reflect.Value{reflect.ValueOf(f), reflect.ValueOf(h), reflect.ValueOf(data)})[0]
	return ret.Interface().(Outcome)
}

func (m marshalledFunc) decisionFunc(f *FSMContext, h *swf.HistoryEvent, data interface{}) *swf.Decision {
	ret := m.v.Call([]reflect.Value{reflect.ValueOf(f), reflect.ValueOf(h), reflect.ValueOf(data)})[0]
	return ret.Interface().(*swf.Decision)
}

func (m marshalledFunc) multiDecisionFunc(f *FSMContext, h *swf.HistoryEvent, data interface{}) []*swf.Decision {
	ret := m.v.Call([]reflect.Value{reflect.ValueOf(f), reflect.ValueOf(h), reflect.ValueOf(data)})[0]
	return ret.Interface().([]*swf.Decision)
}

func (m marshalledFunc) stateFunc(f *FSMContext, h *swf.HistoryEvent, data interface{}) {
	m.v.Call([]reflect.Value{reflect.ValueOf(f), reflect.ValueOf(h), reflect.ValueOf(data)})
}

func (m marshalledFunc) predicateFunc(data interface{}) bool {
	return m.v.Call([]reflect.Value{reflect.ValueOf(data)})[0].Interface().(bool)
}

func typeCheck(typedFunc interface{}, in []string, out []string) {
	t := reflect.TypeOf(typedFunc)
	if reflect.Func != t.Kind() {
		panic(fmt.Sprintf("kind was %v, not Func", t.Kind()))
	}
	if len(in) != t.NumIn() {
		panic(fmt.Sprintf(
			"input arity was %v, not %v",
			t.NumIn(), len(in),
		))
	}

	for i, rt := range in {
		if rt != t.In(i).String() {
			panic(fmt.Sprintf(
				"type of argument %v was %v, not %v",
				i, t.In(i), rt,
			))
		}
	}

	if len(out) != t.NumOut() {
		panic(fmt.Sprintf(
			"number of return values was %v, not %v",
			t.NumOut(), len(out),
		))
	}

	for i, rt := range out {
		if rt != t.Out(i).String() {
			panic(fmt.Sprintf(
				"type of return value %v was %v, not %v",
				i, t.Out(i), rt,
			))
		}
	}
}

// OnStarted builds a composed decider that fires on swf.EventTypeWorkflowExecutionStarted.
func OnStarted(deciders ...Decider) Decider {
	return func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
		switch *h.EventType {
		case swf.EventTypeWorkflowExecutionStarted:
			logf(ctx, "at=on-started")
			return NewComposedDecider(deciders...)(ctx, h, data)
		}
		return ctx.Pass()
	}
}

func OnContinueFailed(deciders ...Decider) Decider {
	return func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
		switch *h.EventType {
		case swf.EventTypeContinueAsNewWorkflowExecutionFailed:
			logf(ctx, "at=on-continuefailed")
			return NewComposedDecider(deciders...)(ctx, h, data)
		}
		return ctx.Pass()
	}
}

// OnChildStarted builds a composed decider that fires on swf.EventTypeChildWorkflowExecutionStarted.
func OnChildStarted(deciders ...Decider) Decider {
	return func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
		switch *h.EventType {
		case swf.EventTypeChildWorkflowExecutionStarted:
			logf(ctx, "at=on-child-started")
			return NewComposedDecider(deciders...)(ctx, h, data)
		}
		return ctx.Pass()
	}
}

// OnData builds a composed decider that fires on when the PredicateFunc is satisfied.
func OnData(predicate PredicateFunc, deciders ...Decider) Decider {
	return func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
		if predicate(data) {
			logf(ctx, "at=on-data")
			return NewComposedDecider(deciders...)(ctx, h, data)
		}
		return ctx.Pass()
	}
}

// OnSignalsReceived builds a composed decider that fires on when one of the matching signal is received.
func OnSignalsReceived(signalNames []string, deciders ...Decider) Decider {
	return func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
		switch *h.EventType {
		case swf.EventTypeWorkflowExecutionSignaled:
			for _, signalName := range signalNames {
				if *h.WorkflowExecutionSignaledEventAttributes.SignalName == signalName {
					logf(ctx, "at=on-signal-received")
					return NewComposedDecider(deciders...)(ctx, h, data)
				}
			}
		}
		return ctx.Pass()
	}
}

// OnSignalReceived builds a composed decider that fires on when a matching signal is received.
func OnSignalReceived(signalName string, deciders ...Decider) Decider {
	return OnSignalsReceived([]string{signalName}, deciders...)
}

// OnSignalSent builds a composed decider that fires on when a matching signal is received.
func OnSignalSent(signalName string, deciders ...Decider) Decider {
	return func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
		switch *h.EventType {
		case swf.EventTypeExternalWorkflowExecutionSignaled:
			// if we find a good signal info with matching signal, we have matched workflowId and signalId so fire deciders.
			info := ctx.SignalInfo(h)
			if info != nil && info.SignalName == signalName {
				logf(ctx, "at=on-signal-sent")
				return NewComposedDecider(deciders...)(ctx, h, data)
			}
		}
		return ctx.Pass()
	}
}

// OnTimerFired builds a composed decider that fires on when a matching timer is fired.
func OnTimerFired(timerId string, deciders ...Decider) Decider {
	return func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
		switch *h.EventType {
		case swf.EventTypeTimerFired:
			if *h.TimerFiredEventAttributes.TimerId == timerId {
				logf(ctx, "at=on-timer-fired")
				return NewComposedDecider(deciders...)(ctx, h, data)
			}
		}
		return ctx.Pass()
	}
}

// OnSignalFailed builds a composed decider that fires on when a matching signal fails.
func OnSignalFailed(signalName string, deciders ...Decider) Decider {
	return func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
		switch *h.EventType {
		case swf.EventTypeSignalExternalWorkflowExecutionFailed:
			// if we find a good signal info with matching signal, we have matched workflowId and signalId so fire deciders.
			info := ctx.SignalInfo(h)
			if info != nil && info.SignalName == signalName {
				logf(ctx, "at=on-signal-failed")
				return NewComposedDecider(deciders...)(ctx, h, data)
			}
		}
		return ctx.Pass()
	}
}

func OnActivityEvents(activityName string, eventTypes []string, deciders ...Decider) Decider {
	return func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
		for _, eventType := range eventTypes {
			info := ctx.ActivityInfo(h)
			if info != nil && *h.EventType == eventType && *ctx.ActivityInfo(h).Name == activityName {
				logf(ctx, "at=on-activity-event")
				return NewComposedDecider(deciders...)(ctx, h, data)
			}
		}
		return ctx.Pass()
	}
}

// OnActivityStarted builds a composed decider that fires when a matching activity starts.
func OnActivityStarted(activityName string, deciders ...Decider) Decider {
	return OnActivityEvents(activityName, []string{
		swf.EventTypeActivityTaskStarted,
	}, deciders...)
}

// OnActivityCompleted builds a composed decider that fires when a matching activity completes.
func OnActivityCompleted(activityName string, deciders ...Decider) Decider {
	return OnActivityEvents(activityName, []string{
		swf.EventTypeActivityTaskCompleted,
	}, deciders...)
}

// OnActivityFailed builds a composed decider that fires when a matching activity fails.
func OnActivityFailed(activityName string, deciders ...Decider) Decider {
	return OnActivityEvents(activityName, []string{
		swf.EventTypeActivityTaskFailed,
		swf.EventTypeScheduleActivityTaskFailed,
	}, deciders...)
}

// OnActivityTimedOut builds a composed decider that fires when a matching activity times out.
func OnActivityTimedOut(activityName string, deciders ...Decider) Decider {
	return OnActivityEvents(activityName, []string{
		swf.EventTypeActivityTaskTimedOut,
	}, deciders...)
}

// OnActivityCanceled builds a composed decider that fires when a matching activity is canceled.
func OnActivityCanceled(activityName string, deciders ...Decider) Decider {
	return OnActivityEvents(activityName, []string{
		swf.EventTypeActivityTaskCanceled,
	}, deciders...)
}

// OnActivityFailedTimedOutCanceled builds a composed decider that fires when a matching activity fails, times out, or is canceled.
func OnActivityFailedTimedOutCanceled(activityName string, deciders ...Decider) Decider {
	return OnActivityEvents(activityName, []string{
		swf.EventTypeActivityTaskFailed,
		swf.EventTypeActivityTaskTimedOut,
		swf.EventTypeActivityTaskCanceled,
		swf.EventTypeScheduleActivityTaskFailed,
	}, deciders...)
}

func OnWorkflowCancelRequested(deciders ...Decider) Decider {
	return func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
		switch *h.EventType {
		case swf.EventTypeWorkflowExecutionCancelRequested:
			logf(ctx, "at=on-wokrflow-execution-cancel-requested")
			return NewComposedDecider(deciders...)(ctx, h, data)
		}
		return ctx.Pass()
	}
}

func OnExternalWorkflowExecutionCancelRequested(deciders ...Decider) Decider {
	return func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
		switch *h.EventType {
		case swf.EventTypeExternalWorkflowExecutionCancelRequested:
			logf(ctx, "at=on-external-workflow-execution-cancel-requested")
			return NewComposedDecider(deciders...)(ctx, h, data)
		}
		return ctx.Pass()
	}
}

func OnRequestCancelExternalWorkflowExecutionFailed(deciders ...Decider) Decider {
	return func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
		switch *h.EventType {
		case swf.EventTypeRequestCancelExternalWorkflowExecutionFailed:
			logf(ctx, "at=on-request-cancel-external-workflow-execution-failed")
			return NewComposedDecider(deciders...)(ctx, h, data)
		}
		return ctx.Pass()
	}
}

// AddDecision adds a single decision to a ContinueDecider outcome
func AddDecision(decisionFn DecisionFunc) Decider {
	return func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
		decisions := ctx.EmptyDecisions()
		d := decisionFn(ctx, h, data)
		logf(ctx, "at=decide")
		decisions = append(decisions, d)
		return ctx.ContinueDecider(data, decisions)
	}
}

// AddDecisions adds decisions to a ContinueDecider outcome
func AddDecisions(signalFn MultiDecisionFunc) Decider {
	return func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
		decisions := ctx.EmptyDecisions()
		ds := signalFn(ctx, h, data)
		logf(ctx, "at=decide-many")
		decisions = append(decisions, ds...)
		return ctx.ContinueDecider(data, decisions)
	}
}

// UpdateState allows you to modicy the state data without generating decisions.
func UpdateState(updateFunc StateFunc) Decider {
	return func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
		logf(ctx, "at=update-state")
		updateFunc(ctx, h, data)
		return ctx.ContinueDecider(data, ctx.EmptyDecisions())
	}
}

// Transition transitions the FSM to a new state, and terminates the decdier.
func Transition(toState string) Decider {
	return func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
		logf(ctx, "at=transition")
		return ctx.Goto(toState, data, ctx.EmptyDecisions())
	}
}

// CompleteWorkflow completes the workflow
func CompleteWorkflow() Decider {
	return func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
		Log.Printf("at=complete-workflow workflowId=%s", LS(ctx.WorkflowId))
		return ctx.CompleteWorkflow(data)
	}
}

// CancelWorkflow cancels the workflow
func CancelWorkflow(details *string) Decider {
	return func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
		Log.Printf("at=cancel-workflow workflowId=%s", LS(ctx.WorkflowId))
		return ctx.CancelWorkflow(data, details)
	}
}

// Stay keeps the fsm in the same state, and terminates the decider.
func Stay() Decider {
	return func(ctx *FSMContext, h *swf.HistoryEvent, data interface{}) Outcome {
		logf(ctx, "at=stay")
		return ctx.Stay(data, ctx.EmptyDecisions())
	}
}

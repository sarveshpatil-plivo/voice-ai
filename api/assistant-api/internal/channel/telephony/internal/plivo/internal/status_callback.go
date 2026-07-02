// Copyright (c) 2023-2025 RapidaAI
// Author: Sarvesh Patil <sarvesh.patil@plivo.com>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_plivo

import (
	"strings"
	"time"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/pkg/validator"
)

// StatusCallback is a parsed Plivo answer/hangup webhook, posted as
// application/x-www-form-urlencoded fields to the number's hangup_url. CallUUID,
// CallStatus, and Event are the reliable fields that drive terminal/failure
// detection; Duration/HangupCause/HangupSource/TotalCost are read best-effort and
// may be absent depending on Plivo configuration/version.
type StatusCallback struct {
	Event        string
	ChannelUUID  string
	Duration     *time.Duration
	Price        string
	HangupCause  string
	HangupSource string
	RawPayload   string
	Payload      utils.Option
}

// NewStatusCallback parses the collected Plivo webhook fields into a StatusCallback.
// The event is taken from CallStatus (falling back to Event); the call identifier
// from CallUUID; and duration from Duration (falling back to BillDuration). It
// returns ErrStatusCallbackStatusMissing when no status/event field is present.
func NewStatusCallback(eventDetails utils.Option, rawCallbackPayload string) (*StatusCallback, error) {
	event, _ := eventDetails.GetString("CallStatus")
	if !validator.NotBlank(event) {
		event, _ = eventDetails.GetString("Event")
	}
	if !validator.NotBlank(event) {
		return nil, ErrStatusCallbackStatusMissing
	}

	channelUUID, _ := eventDetails.GetString("CallUUID")

	duration, err := eventDetails.GetDuration("Duration")
	if err != nil {
		duration, err = eventDetails.GetDuration("BillDuration")
	}
	var durationPtr *time.Duration
	if err == nil {
		durationPtr = utils.Ptr(duration)
	}

	price, _ := eventDetails.GetString("TotalCost")
	if !validator.NotBlank(price) {
		price, _ = eventDetails.GetString("Price")
	}
	hangupCause, _ := eventDetails.GetString("HangupCause")
	hangupSource, _ := eventDetails.GetString("HangupSource")

	return &StatusCallback{
		Event:        event,
		ChannelUUID:  channelUUID,
		Duration:     durationPtr,
		Price:        price,
		HangupCause:  hangupCause,
		HangupSource: hangupSource,
		RawPayload:   rawCallbackPayload,
		Payload:      eventDetails,
	}, nil
}

// StatusInfo converts the parsed callback into the provider-agnostic StatusInfo,
// marking the call completed on a clean hangup and attaching a StatusError when
// the callback represents a failure.
func (s *StatusCallback) StatusInfo() *internal_type.StatusInfo {
	callbackFailed := s.Failed()
	statusInfo := &internal_type.StatusInfo{
		Event:       s.Event,
		ChannelUUID: s.ChannelUUID,
		Completed:   s.completed() && !callbackFailed,
		Duration:    s.Duration,
		Price:       s.Price,
		RawPayload:  s.RawPayload,
		Payload:     s.Payload,
	}
	if callbackFailed {
		statusInfo.Error = &internal_type.StatusError{Error: "failed", Reason: s.FailureReason()}
	}
	return statusInfo
}

// completed reports whether the callback represents a terminated call.
func (s *StatusCallback) completed() bool {
	eventLower := strings.ToLower(s.Event)
	return eventLower == "completed" ||
		eventLower == "hangup" ||
		validator.NotBlank(s.HangupCause)
}

// Failed reports whether the callback represents a failed call. A call is failed
// when the status is busy/no-answer/failed/cancelled, or when the hangup cause is
// anything other than a clean NORMAL_CLEARING.
func (s *StatusCallback) Failed() bool {
	eventLower := strings.ToLower(s.Event)
	failed := eventLower == "failed" ||
		eventLower == "busy" ||
		eventLower == "no-answer" ||
		eventLower == "no_answer" ||
		eventLower == "timeout" ||
		eventLower == "canceled" ||
		eventLower == "cancelled"
	if validator.NotBlank(s.HangupCause) {
		causeLower := strings.ToLower(s.HangupCause)
		failed = failed || (causeLower != "normal_clearing" && causeLower != "normal clearing")
	}
	return failed
}

// FailureReason returns the most specific human-readable failure reason available.
func (s *StatusCallback) FailureReason() string {
	if validator.NotBlank(s.HangupCause) {
		return s.HangupCause
	}
	if validator.NotBlank(s.HangupSource) {
		return s.HangupSource
	}
	return s.Event
}

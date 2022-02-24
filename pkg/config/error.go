package config

import "errors"

var (
	ErrDuplicateMACAddress          = errors.New("duplicate MAC address")
	ErrConnectionDoesNotExist       = errors.New("connection with this MAC address does not exist")
	ErrCommunityDoesNotExist        = errors.New("community with this name does not exist")
	ErrAlreadyApplied               = errors.New("cannot apply multiple times")
	ErrCouldNotUnmarshalJSON        = errors.New("could not unmarshal JSON")
	ErrCouldNotHandleApplication    = errors.New("could not handle application")
	ErrInvalidCommunityOrMACAddress = errors.New("invalid community or MAC address")
	ErrCouldNotHandleReady          = errors.New("could not handle ready")
	ErrInvalidMACAddress            = errors.New("invalid MAC address")
	ErrCouldNotHandleExited         = errors.New("could not handle exited")
	ErrMACAddressRejected           = errors.New("MAC address rejected")
	ErrUnknownMessageType           = errors.New("unknown message type")
	ErrFingerprintDidNotMatch       = errors.New("fingerprint did not match")
	ErrManualVerificationFailed     = errors.New("manual fingerprint verification failed")
	ErrCouldNotReadKnownHosts       = errors.New("could not read known hosts file")
	ErrNoFingerprintFound           = errors.New("could not find fingerprint for address")
	ErrCouldNotGetUserInput         = errors.New("could not get user input")
	ErrKnownHostsSyntax             = errors.New("syntax error in known hosts")
	ErrAlreadyOpened                = errors.New("already opened")
)

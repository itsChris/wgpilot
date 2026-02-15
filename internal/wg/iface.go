//go:build linux

package wg

import (
	"errors"
	"fmt"
	"strings"

	"github.com/vishvananda/netlink"
)

// netlinkManager implements LinkManager using vishvananda/netlink.
type netlinkManager struct{}

// NewLinkManager creates a new LinkManager backed by netlink.
func NewLinkManager() LinkManager {
	return &netlinkManager{}
}

func (m *netlinkManager) CreateWireGuardLink(name string) error {
	la := netlink.NewLinkAttrs()
	la.Name = name
	link := &netlink.GenericLink{
		LinkAttrs: la,
		LinkType:  "wireguard",
	}
	return netlink.LinkAdd(link)
}

func (m *netlinkManager) DeleteLink(name string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return fmt.Errorf("get link %s: %w", name, err)
	}
	return netlink.LinkDel(link)
}

func (m *netlinkManager) SetLinkUp(name string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return fmt.Errorf("get link %s: %w", name, err)
	}
	return netlink.LinkSetUp(link)
}

func (m *netlinkManager) SetLinkDown(name string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return fmt.Errorf("get link %s: %w", name, err)
	}
	return netlink.LinkSetDown(link)
}

func (m *netlinkManager) AddAddress(linkName string, addr string) error {
	link, err := netlink.LinkByName(linkName)
	if err != nil {
		return fmt.Errorf("get link %s: %w", linkName, err)
	}
	nlAddr, err := netlink.ParseAddr(addr)
	if err != nil {
		return fmt.Errorf("parse address %s: %w", addr, err)
	}
	return netlink.AddrAdd(link, nlAddr)
}

func (m *netlinkManager) ListAddresses(linkName string) ([]string, error) {
	link, err := netlink.LinkByName(linkName)
	if err != nil {
		return nil, fmt.Errorf("get link %s: %w", linkName, err)
	}
	addrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		return nil, fmt.Errorf("list addresses for %s: %w", linkName, err)
	}
	result := make([]string, len(addrs))
	for i, a := range addrs {
		result[i] = a.IPNet.String()
	}
	return result, nil
}

func (m *netlinkManager) LinkExists(name string) (bool, error) {
	_, err := netlink.LinkByName(name)
	if err == nil {
		return true, nil
	}

	// Check for known "not found" error types
	var lnfe netlink.LinkNotFoundError
	if errors.As(err, &lnfe) {
		return false, nil
	}
	// Fallback: match error message
	if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no such device") {
		return false, nil
	}

	return false, fmt.Errorf("check link %s: %w", name, err)
}

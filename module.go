// GlueGetter - Does things with mail
//
// Copyright 2015 Dolf Schimmel, Freeaqingme.
//
// This Source Code Form is subject to the terms of the two-clause BSD license.
// For its contents, please refer to the LICENSE file.
//
package main

func moduleGetResponse(policyRequest map[string]string) string {

	if Config.Quotas.Enabled {
		return quotasIsAllowed(policyRequest)
	}

	return "ACTION=dunno"
}

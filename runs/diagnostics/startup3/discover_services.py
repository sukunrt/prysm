#!/usr/bin/env python3
import json
import sys

wanted = {"bn-1", "bn-3", "vc-3"}


def discover(items):
    out = {}
    for item in items:
        labels = (item.get("Config") or {}).get("Labels") or {}
        service = labels.get("com.kurtosistech.id")
        if service not in wanted:
            continue
        if service in out:
            raise ValueError(f"duplicate service: {service}")
        networks = (item.get("NetworkSettings") or {}).get("Networks") or {}
        addresses = [value["IPAddress"] for value in networks.values() if value.get("IPAddress")]
        if len(addresses) != 1:
            raise ValueError(f"expected one IP for {service}")
        out[service] = {"container_id": item["Id"], "ip": addresses[0]}
    if set(out) != wanted:
        raise ValueError(f"missing services: {sorted(wanted-set(out))}")
    return out


if __name__ == "__main__":
    with open(sys.argv[1]) as source:
        result = discover(json.load(source))
    with open(sys.argv[2], "w") as destination:
        json.dump(result, destination, indent=2)

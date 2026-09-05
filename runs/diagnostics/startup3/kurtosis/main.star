def _ports(items):
    result = {}
    for name, number, transport, protocol in items:
        if protocol == "":
            result[name] = PortSpec(number=number, transport_protocol=transport)
        else:
            result[name] = PortSpec(
                number=number,
                transport_protocol=transport,
                application_protocol=protocol,
            )
    return result


def _geth_cmd(index):
    return [
        "--override.genesis=/bundle/network-configs/genesis.json",
        "--verbosity=3",
        "--datadir=/data/geth",
        "--http",
        "--http.addr=0.0.0.0",
        "--http.port=8545",
        "--http.vhosts=*",
        "--http.corsdomain=*",
        "--http.api=admin,engine,net,eth,web3,debug,txpool",
        "--authrpc.port=8551",
        "--authrpc.addr=0.0.0.0",
        "--authrpc.vhosts=*",
        "--authrpc.jwtsecret=/bundle/jwt/jwtsecret",
        "--syncmode=full",
        "--rpc.allow-unprotected-txs",
        "--metrics",
        "--metrics.addr=0.0.0.0",
        "--metrics.port=9001",
        "--discovery.port=30303",
        "--port=30303",
        "--discovery.v4=false",
        "--discovery.v5=true",
        "--miner.gasprice=1",
        "--miner.gaslimit=200000000",
    ]


def _beacon_cmd(index, bootstrap_enr, ledger, execution_endpoint = ""):
    if execution_endpoint == "":
        execution_endpoint = "http://el-{0}:8551".format(index)
    cmd = [
        "--accept-terms-of-use=true",
        "--datadir=/data/prysm/beacon",
        "--rpc-host=0.0.0.0",
        "--rpc-port=4000",
        "--http-host=0.0.0.0",
        "--http-cors-domain=*",
        "--http-port=3500",
        "--p2p-tcp-port=13000",
        "--p2p-udp-port=12000",
        "--p2p-quic-port=13000",
        "--p2p-host-ip=KURTOSIS_IP_ADDR_PLACEHOLDER",
        "--p2p-colocation-whitelist=0.0.0.0/0,::/0",
        "--min-sync-peers=0",
        "--verbosity=info",
        "--slots-per-archive-point=32",
        "--suggested-fee-recipient=0x8943545177806ED17B9F23F0a21ee5948eCaa776",
        "--disable-monitoring=false",
        "--monitoring-host=0.0.0.0",
        "--monitoring-port=8080",
        "--pprof",
        "--pprofaddr=0.0.0.0",
        "--pprofport=6060",
        "--execution-endpoint=" + execution_endpoint,
        "--jwt-secret=/bundle/jwt/jwtsecret",
        "--subscribe-all-data-subnets=true",
        "--subscribe-all-subnets",
        "--p2p-static-id=true",
        "--chain-config-file=/bundle/network-configs/config.yaml",
        "--genesis-state=/bundle/network-configs/genesis.ssz",
        "--contract-deployment-block=0",
    ]
    if bootstrap_enr != "":
        cmd.append("--bootstrap-node=" + bootstrap_enr)
    if ledger:
        cmd.append("--goldfish-vote-ledger")
    return cmd


def run(plan, args):
    bundle_path = args.get("bundle_path", "")
    if bundle_path == "":
        fail("bundle_path is required")
    if args.get("slots_per_round", 4) != 4:
        fail("this diagnostic requires slots_per_round=4 in config.yaml")
    ledger = args.get("ledger", False)
    genesis_count_ablation = args.get("genesis_count_ablation", False)
    engine_snooper = args.get("engine_snooper", False)
    startup_engine_probe = args.get("startup_engine_probe", False)
    bn_env = {"GOMAXPROCS": "4", "PRYSM_STARTUP_DIAGNOSTIC": "1"}
    if genesis_count_ablation:
        # Understood only by the local diagnostic image. This deliberately
        # changes the genesis active-validator-count path and is never a
        # production-compatible setting.
        bn_env["PRYSM_DIAGNOSTIC_GENESIS_COUNT_ABLATION"] = "1"
    bundle = plan.upload_files(src=bundle_path, name="startup3-bundle")
    mounts = {"/bundle": bundle}

    el_ports = _ports(
        [
            ("rpc", 8545, "TCP", "http"),
            ("engine", 8551, "TCP", "http"),
            ("metrics", 9001, "TCP", "http"),
        ]
    )
    bn_ports = _ports(
        [
            ("rest", 3500, "TCP", "http"),
            ("rpc", 4000, "TCP", "grpc"),
            ("metrics", 8080, "TCP", "http"),
            ("pprof", 6060, "TCP", "http"),
            ("p2p-tcp", 13000, "TCP", ""),
            ("p2p-quic", 13000, "UDP", ""),
            ("p2p-udp", 12000, "UDP", ""),
        ]
    )
    for index in range(1, 4):
        plan.add_service(
            name="el-{0}".format(index),
            config=ServiceConfig(
                image="ethpandaops/geth:glamsterdam-devnet-8",
                cmd=_geth_cmd(index),
                ports=el_ports,
                files=mounts,
            ),
        )

    if engine_snooper:
        plan.add_service(
            name="snooper-engine-3",
            config=ServiceConfig(
                image="ethpandaops/rpc-snooper:v0.0.21",
                cmd=[
                    "--bind-address=0.0.0.0",
                    "--port=8561",
                    "--no-api",
                    "--jwt-secret=/bundle/jwt/jwtsecret",
                    "http://el-3:8551",
                ],
                ports=_ports([("engine", 8561, "TCP", "http")]),
                files=mounts,
            ),
        )

    plan.add_service(
        name="bn-1",
        config=ServiceConfig(
            image="prysm-beacon-chain:startup-repro",
            cmd=_beacon_cmd(1, "", ledger),
            private_ip_address_placeholder="KURTOSIS_IP_ADDR_PLACEHOLDER",
            env_vars=bn_env,
            ports=bn_ports,
            files=mounts,
            ready_conditions=ReadyCondition(
                recipe=GetHttpRequestRecipe(
                    port_id="rest", endpoint="/eth/v1/node/identity"
                ),
                field="code",
                assertion="==",
                target_value=200,
            ),
        ),
    )
    identity = plan.request(
        recipe=GetHttpRequestRecipe(
            port_id="rest",
            endpoint="/eth/v1/node/identity",
            extract={"enr": ".data.enr"},
        ),
        service_name="bn-1",
    )
    bootstrap_enr = identity["extract.enr"]
    for index in range(2, 4):
        execution_endpoint = ""
        node_env = bn_env
        if index == 3 and engine_snooper:
            execution_endpoint = "http://snooper-engine-3:8561"
        if index == 3 and startup_engine_probe:
            node_env = dict(bn_env)
            node_env["PRYSM_STARTUP_ENGINE_PROBE"] = "1"
        plan.add_service(
            name="bn-{0}".format(index),
            config=ServiceConfig(
                image="prysm-beacon-chain:startup-repro",
                cmd=_beacon_cmd(index, bootstrap_enr, ledger, execution_endpoint),
                private_ip_address_placeholder="KURTOSIS_IP_ADDR_PLACEHOLDER",
                env_vars=node_env,
                ports=bn_ports,
                files=mounts,
            ),
        )

    vc_ports = _ports(
        [
            ("http", 5056, "TCP", "http"),
            ("metrics", 8080, "TCP", "http"),
            ("pprof", 6061, "TCP", "http"),
        ]
    )
    plan.add_service(
        name="vc-3",
        config=ServiceConfig(
            image="prysm-validator:startup-repro",
            cmd=[
                "--accept-terms-of-use=true",
                "--datadir=/data/prysm/validator",
                "--verbosity=info",
                "--chain-config-file=/bundle/network-configs/config.yaml",
                "--suggested-fee-recipient=0x8943545177806ED17B9F23F0a21ee5948eCaa776",
                "--disable-monitoring=false",
                "--monitoring-host=0.0.0.0",
                "--monitoring-port=8080",
                "--beacon-rpc-provider=bn-3:4000",
                "--wallet-dir=/bundle/validator-keys/prysm",
                "--wallet-password-file=/bundle/prysm-password/prysm-password.txt",
                "--suggested-gas-limit=200000000",
                "--decoupled-ffg-vote-at-slot-start",
                "--pprof",
                "--pprofaddr=0.0.0.0",
                "--pprofport=6061",
                "--rpc",
                "--http-port=5056",
                "--http-host=0.0.0.0",
                "--keymanager-token-file=/bundle/keymanager/keymanager.txt",
            ],
            env_vars={"GOMAXPROCS": "4", "PRYSM_STARTUP_DIAGNOSTIC": "1"},
            ports=vc_ports,
            files=mounts,
        ),
    )
    plan.print("Started three EL/BN pairs and the 596-key validator on bn-3")

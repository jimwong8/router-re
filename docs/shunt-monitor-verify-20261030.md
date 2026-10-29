# Shunt Monitor Verify 2026-10-30

## GET /api/v1/policyroute/hits -> 200
```json
{"count":1,"hits":[{"hits":0,"rule":{"priority":100,"from":"10.100.1.0/24","to":"","fwmark":0,"table":100,"iif":""}}]}
```

## GET /api/v1/lb/status -> 200 (per_member_rx_tx)
```json
{"per_member_rx_tx":{"wan1":{"name":"ens18","rx_bytes":151767629019,"rx_packets":181201714,"rx_errors":0,"tx_bytes":203014079504,"tx_packets":195698319,"tx_errors":0}}}
```

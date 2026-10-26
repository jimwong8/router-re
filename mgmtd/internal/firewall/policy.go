package firewall

type Rule struct {
    Chain  string
    Src    string
    Dst    string
    Proto  string
    DPort  string
    Action string
}

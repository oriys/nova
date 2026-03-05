package zenith

import (
	"strings"

	"google.golang.org/grpc/resolver"
)

// staticResolverBuilder is a gRPC resolver that returns a fixed set of
// addresses.  Used when Zenith is configured with comma-separated backend
// addresses (e.g. "nova1:8081,nova2:8081") for round-robin load balancing.
type staticResolverBuilder struct {
	addrs []resolver.Address
}

func newStaticResolver(addrs []string) resolver.Builder {
	ra := make([]resolver.Address, 0, len(addrs))
	for _, a := range addrs {
		a = strings.TrimSpace(a)
		if a != "" {
			ra = append(ra, resolver.Address{Addr: a})
		}
	}
	return &staticResolverBuilder{addrs: ra}
}

func (b *staticResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (resolver.Resolver, error) {
	cc.UpdateState(resolver.State{Addresses: b.addrs})
	return &staticResolverInstance{}, nil
}

func (b *staticResolverBuilder) Scheme() string { return "static" }

type staticResolverInstance struct{}

func (*staticResolverInstance) ResolveNow(resolver.ResolveNowOptions) {}
func (*staticResolverInstance) Close()                                {}

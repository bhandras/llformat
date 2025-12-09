package main

import (
	"context"
	"time"
)

// =============================================================================
// SECTION 1: Method Chains
// =============================================================================

// Example 1: Simple method chain that exceeds line limit
func methodChain1() {
	result := client.WithTimeout(30*time.Second).
		WithRetry(3).
		WithHeaders(headers).
		Execute(ctx, request)
	_ = result
}

// Example 2: Method chain with long method names
func methodChain2() {
	result := builder.SetVeryLongConfigurationOption(true).
		EnableAnotherFeatureFlag(false).
		ConfigureAdvancedSettings(opts).
		Build()
	_ = result
}

// Example 3: Method chain on receiver expression (from example.go_ line
// 468-471)
func methodChain3(update *NodeUpdate) {
	pubKeyStr := string(update.IdentityKey.SerializeCompressed())
	_ = pubKeyStr
}

// Example 4: Deeply nested method chain
func methodChain4() {
	result := factory.CreateBuilder().
		WithOption1(val1).
		WithOption2(val2).
		WithOption3(val3).
		WithOption4(val4).
		Finalize().
		Build()
	_ = result
}

// Example 5: Method chain with struct literal argument
func methodChain5() {
	result := client.Configure(
		Config{
			Timeout:        30 * time.Second,
			MaxConnections: 3,
		},
	).Execute(ctx, nil)
	_ = result
}

// Example 6: Method chain returning error
func methodChain6() error {
	return processor.Initialize(ctx).
		LoadConfig(configPath).
		ValidateInputs(inputs).
		ProcessAll().
		Finalize()
}

// =============================================================================
// SECTION 2: Struct Literals
// =============================================================================

// Example 7: Long struct literal in assignment
func structLiteral1() {
	cfg := Config{
		Host:           "localhost",
		Port:           8080,
		Timeout:        30 * time.Second,
		MaxConnections: 100,
		EnableTLS:      true,
	}
	_ = cfg
}

// Example 8: Struct literal as function argument
func structLiteral2() {
	server := NewServer(
		Config{
			Host:           "localhost",
			Port:           8080,
			Timeout:        30 * time.Second,
			MaxConnections: 100,
			EnableTLS:      true,
			CertPath:       "/path/to/cert",
		})
	_ = server
}

// Example 9: Nested struct literals
func structLiteral3() {
	opts := Options{
		Server: ServerConfig{
			Host: "localhost",
			Port: 8080,
		},
		Client: ClientConfig{
			Timeout: 30 * time.Second,
			Retries: 3,
		},
	}
	_ = opts
}

// Example 10: Struct literal with anonymous function field (from example.go_)
func structLiteral4() {
	handler := Handler{
		OnSuccess: func(result string) {
			processResult(result)
		},
		OnError: func(err error) {
			logError(err)
		},
	}
	_ = handler
}

// Example 11: Large struct literal (similar to example.go_ feature manager)
func structLiteral5() {
	mgr := FeatureManager{
		EnableFeatureA: true,
		EnableFeatureB: false,
		EnableFeatureC: true,
		EnableFeatureD: false,
		Timeout:        30 * time.Second,
		MaxRetries:     5,
	}
	_ = mgr
}

// =============================================================================
// SECTION 3: Make Calls
// =============================================================================

// Example 12: Make with capacity (from example.go_ line 482-483)
func makeCall1(update *NodeUpdate) {
	addrs := make([]*NetAddress, 0, len(update.Addresses))
	_ = addrs
}

// Example 13: Make with long type
func makeCall2() {
	results := make(map[string]*VeryLongTypeName, 100)
	_ = results
}

// Example 14: Make in append
func makeCall3(items []Item) {
	result := append(make([]ProcessedItem, 0, len(items)), ProcessedItem{})
	_ = result
}

// =============================================================================
// SECTION 4: Defer Statements
// =============================================================================

// Example 15: Defer with function literal (from example.go_ line 450-453)
func deferExample1(sub *Subscription) {
	defer func() {
		sub.Cancel()
		cleanup()
	}()
	doWork()
}

// Example 16: Defer with method call chain
func deferExample2(conn *Connection) {
	defer conn.Transaction().Rollback()
	doTransaction()
}

// =============================================================================
// SECTION 5: Long Assignments
// =============================================================================

// Example 17: Assignment with long selector expression (from example.go_ line
// 1860-1861)
func assignment1(routerCfg *RouterConfig) {
	routerCfg.ProbabilityEstimatorType = routingpkg.AprioriEstimatorName
}

// Example 18: Assignment with long qualified name
func assignment2() {
	estimator := missionctrl.NewProbabilityEstimator(
		missionctrl.DefaultConfig(),
	)
	_ = estimator
}

// Example 19: Multiple assignments on same line (should stay separate)
func assignment3(a, b, c, d, e, f, g, h, i, j, k, l, m, n, o, p int) {
	sum := a + b + c + d + e + f + g + h + i + j + k + l + m + n + o + p
	_ = sum
}

// =============================================================================
// SECTION 6: Append Patterns
// =============================================================================

// Example 20: Append with struct literal (from example.go_ line 486-492)
func appendExample1(addrs []*NetAddress, update *NodeUpdate,
	net Network) []*NetAddress {

	return append(
		addrs, &NetAddress{
			IdentityKey: update.IdentityKey,
			Address:     update.Address,
			ChainNet:    net,
		},
	)
}

// Example 21: Append with multiple elements
func appendExample2(items []string) []string {
	return append(
		items, "item1", "item2", "item3", "item4", "item5", "item6",
		"item7", "item8",
	)
}

// =============================================================================
// SECTION 7: Complex If Conditions (additional cases)
// =============================================================================

// Example 22: If with method call chain in condition
func complexIf1(cache *Cache) {
	if cache.Get(key).IsValid() && cache.Get(key).NotExpired() &&
		cache.Get(key).HasPermission(user) {
		process()
	}
}

// Example 23: If with long type assertion
func complexIf2(val interface{}) {
	if result, ok := val.(map[string]interface{}); ok && len(result) > 0 &&
		result["enabled"] == true {

		process()
	}
}

// =============================================================================
// SECTION 8: Error Handling Patterns
// =============================================================================

// Example 24: Error wrapping with context (from example.go_)
func errorHandling1() error {
	return fmtpkg.Errorf("failed to initialize component %s with config "+
		"%v: %w", componentName, config, err)
}

// Example 25: Multiple error checks in sequence (cleanup pattern from
// example.go_)
func errorHandling2(s *Server) {
	if err := s.chanStatusMgr.Stop(); err != nil {
		srvrLog.Warnf("failed to stop chanStatusMgr: %v", err)
	}
	if err := s.htlcSwitch.Stop(); err != nil {
		srvrLog.Warnf("failed to stop htlcSwitch: %v", err)
	}
	if err := s.interceptableSwitch.Stop(); err != nil {
		srvrLog.Warnf("failed to stop interceptable switch: %v", err)
	}
}

// =============================================================================
// SECTION 9: Switch with Type Assertions
// =============================================================================

// Example 26: Type switch with long cases (from example.go_ line 1858)
func typeSwitch1(est *Estimator) {
	switch c := est.Config().(type) {
	case AprioriConfig:
		handleApriori(c)

	case BimodalConfig:
		handleBimodal(c)

	case ExperimentalLongConfigTypeName:
		handleExperimental(c)
	}
}

// =============================================================================
// SECTION 10: Function Literals as Arguments
// =============================================================================

// Example 27: Function literal with parameters
func funcLiteral1() {
	processFn(
		func(ctx context.Context, data []byte, opts Options) (Result,
			error) {

			return Result{}, nil
		},
	)
}

// Example 28: Multiple function literal arguments
func funcLiteral2() {
	handle(
		func() error {
			return nil
		},
		func() error {
			return nil
		},
		func() error {
			return nil
		},
	)
}

// =============================================================================
// Stub types and functions to make the file compile
// =============================================================================

var (
	ctx           context.Context
	request       interface{}
	headers       interface{}
	opts          interface{}
	val1          interface{}
	val2          interface{}
	val3          interface{}
	val4          interface{}
	configPath    string
	inputs        interface{}
	key           string
	user          interface{}
	componentName string
	config        interface{}
	err           error
)

type clientType struct{}

func (clientType) WithTimeout(time.Duration) clientType {
	return clientType{}
}

func (clientType) WithRetry(int) clientType {
	return clientType{}
}

func (clientType) WithHeaders(interface{}) clientType {

	return clientType{}
}

func (clientType) Execute(context.Context, interface{}) interface{} {

	return nil
}

func (clientType) Configure(Config) clientType {
	return clientType{}
}

var client clientType

type builderType struct{}

func (builderType) SetVeryLongConfigurationOption(bool) builderType {
	return builderType{}
}

func (builderType) EnableAnotherFeatureFlag(bool) builderType {
	return builderType{}
}

func (builderType) ConfigureAdvancedSettings(interface{}) builderType {

	return builderType{}
}

func (builderType) Build() interface{} {

	return nil
}

var builder builderType

type NodeUpdate struct {
	IdentityKey *IdentityKey
	Addresses   []interface{}
	Address     interface{}
}

type IdentityKey struct{}

func (*IdentityKey) SerializeCompressed() []byte {
	return nil
}

type factoryType struct{}

func (factoryType) CreateBuilder() factoryType {
	return factoryType{}
}

func (factoryType) WithOption1(interface{}) factoryType {

	return factoryType{}
}

func (factoryType) WithOption2(interface{}) factoryType {

	return factoryType{}
}

func (factoryType) WithOption3(interface{}) factoryType {

	return factoryType{}
}

func (factoryType) WithOption4(interface{}) factoryType {

	return factoryType{}
}

func (factoryType) Finalize() factoryType {
	return factoryType{}
}

func (factoryType) Build() interface{} {

	return nil
}

var factory factoryType

type processorType struct{}

func (processorType) Initialize(context.Context) processorType {
	return processorType{}
}

func (processorType) LoadConfig(string) processorType {
	return processorType{}
}

func (processorType) ValidateInputs(interface{}) processorType {

	return processorType{}
}

func (processorType) ProcessAll() processorType {
	return processorType{}
}

func (processorType) Finalize() error {
	return nil
}

var processor processorType

type Config struct {
	Host           string
	Port           int
	Timeout        time.Duration
	MaxConnections int
	EnableTLS      bool
	CertPath       string
}

type ServerConfig struct {
	Host string
	Port int
}

type ClientConfig struct {
	Timeout time.Duration
	Retries int
}

type Options struct {
	Server ServerConfig
	Client ClientConfig
}

type Handler struct {
	OnSuccess func(string)
	OnError   func(error)
}

type FeatureManager struct {
	EnableFeatureA bool
	EnableFeatureB bool
	EnableFeatureC bool
	EnableFeatureD bool
	Timeout        time.Duration
	MaxRetries     int
}

type NetAddress struct {
	IdentityKey *IdentityKey
	Address     interface{}
	ChainNet    Network
}

type Network struct{}

type VeryLongTypeName struct{}
type Item struct{}
type ProcessedItem struct{}

type Subscription struct{}

func (*Subscription) Cancel() {}
func cleanup()                {}
func doWork()                 {}

type Connection struct{}
type Transaction struct{}

func (*Connection) Transaction() *Transaction {
	return nil
}

func (*Transaction) Rollback() {}
func doTransaction()           {}

type RouterConfig struct {
	ProbabilityEstimatorType string
}

var routingpkg = struct {
	AprioriEstimatorName string
}{
	AprioriEstimatorName: "apriori",
}

var missionctrl = struct {
	NewProbabilityEstimator func(interface{}) interface{}
	DefaultConfig           func() interface{}
}{
	NewProbabilityEstimator: func(interface{}) interface{} { return nil },
	DefaultConfig:           func() interface{} { return nil },
}

type Cache struct{}
type CacheEntry struct{}

func (*Cache) Get(string) *CacheEntry {
	return nil
}

func (*CacheEntry) IsValid() bool {
	return false
}

func (*CacheEntry) NotExpired() bool {
	return false
}

func (*CacheEntry) HasPermission(interface{}) bool {

	return false
}

func process()                                                         {}
func processFn(func(context.Context, []byte, Options) (Result, error)) {}
func processResult(string)                                             {}
func logError(error)                                                   {}

func NewServer(Config) *Server {
	return nil
}

type Server struct {
	chanStatusMgr       *Component
	htlcSwitch          *Component
	interceptableSwitch *Component
}

type Component struct{}

func (*Component) Stop() error {
	return nil
}

type logType struct{}

func (logType) Warnf(string, ...interface{}) {}

var srvrLog logType

type Estimator struct{}

func (*Estimator) Config() interface{} {

	return nil
}

type AprioriConfig struct{}
type BimodalConfig struct{}
type ExperimentalLongConfigTypeName struct{}

func handleApriori(interface{})      {}
func handleBimodal(interface{})      {}
func handleExperimental(interface{}) {}

type Result struct{}

func handle(...func() error) {}

var fmtpkg = struct {
	Errorf func(string, ...interface{}) error
}{
	Errorf: func(string, ...interface{}) error { return nil },
}

func main() {}

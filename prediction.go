package kongcompletion

import (
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/posener/complete/v2"
	"github.com/posener/complete/v2/predict"
)

const predictorTag = "predictor"

type options struct {
	predictors   map[string]complete.Predictor
	exitFunc     func(code int)
	errorHandler func(error)
	overrides    map[string]bool
}

// Option is a configuration option for running Register
type Option func(*options)

// WithPredictor use the named predictor
func WithPredictor(name string, predictor complete.Predictor) Option {
	return func(o *options) {
		if o.predictors == nil {
			o.predictors = map[string]complete.Predictor{}
		}
		o.predictors[name] = predictor
	}
}

// WithPredictors use these predictors
func WithPredictors(predictors map[string]complete.Predictor) Option {
	return func(o *options) {
		for k, v := range predictors {
			WithPredictor(k, v)(o)
		}
	}
}

// WithExitFunc the exit command that is run after completions
func WithExitFunc(exitFunc func(code int)) Option {
	return func(o *options) {
		o.exitFunc = exitFunc
	}
}

// WithErrorHandler handle errors with completions
func WithErrorHandler(handler func(error)) Option {
	return func(o *options) {
		o.errorHandler = handler
	}
}

// WithFlagOverrides registers overrides for hidden commands / flags
func WithFlagOverrides(overrides ...map[string]bool) Option {
	allOverrides := make(map[string]bool)
	for _, os := range overrides {
		for k, v := range os {
			allOverrides[k] = v
		}
	}
	return func(o *options) {
		o.overrides = allOverrides
	}
}

func (o *options) Skip(f *kong.Flag) bool {
	doShow, wasSet := o.overrides[f.Name]
	if !wasSet {
		return f.Hidden
	}
	return !doShow
}

func buildOptions(opt ...Option) *options {
	opts := &options{
		predictors: map[string]complete.Predictor{},
	}
	for _, o := range opt {
		o(opts)
	}
	return opts
}

// Command returns a completion Command for a kong parser
func Command(parser *kong.Kong, opt ...Option) (complete.Command, error) {
	opts := buildOptions(opt...)
	if parser == nil || parser.Model == nil {
		return complete.Command{}, nil
	}
	command, err := nodeCommand(parser.Model.Node, opts)
	if err != nil {
		return complete.Command{}, err
	}
	return *command, err
}

// Register configures a kong app for intercepting completions.
func Register(parser *kong.Kong, opt ...Option) {
	if parser == nil {
		return
	}
	opts := buildOptions(opt...)
	errHandler := opts.errorHandler
	if errHandler == nil {
		errHandler = func(err error) {
			parser.Errorf("error running command completion: %v", err)
		}
	}
	exitFunc := opts.exitFunc
	if exitFunc == nil {
		exitFunc = parser.Exit
	}
	cmd, err := Command(parser, opt...)
	if err != nil {
		errHandler(err)
		exitFunc(1)
	}
	cmd.Complete(parser.Model.Name)
	//cmp := complete.New(parser.Model.Name, cmd)
	/*cmp := complete.Complete(parser.Model.Name, cmd)
	cmp.Out = parser.Stdout
	done := cmp.Complete()
	if done {
		exitFunc(0)
	}*/
}

func nodeCommand(node *kong.Node, opts *options) (*complete.Command, error) {
	if node == nil {
		return nil, nil
	}

	cmd := complete.Command{
		Sub:   map[string]*complete.Command{},
		Flags: map[string]complete.Predictor{},
	}

	for _, child := range node.Children {
		if child == nil || child.Hidden {
			continue
		}
		childCmd, err := nodeCommand(child, opts)
		if err != nil {
			return nil, err
		}
		if childCmd != nil {
			cmd.Sub[child.Name] = childCmd
		}
	}

	for _, flag := range node.Flags {
		if flag == nil || opts.Skip(flag) {
			continue
		}
		predictor, err := flagPredictor(flag, opts.predictors)
		if err != nil {
			return nil, err
		}

		cmd.Flags[flag.Name] = predictor
		if flag.Short != 0 {
			cmd.Flags[string(flag.Short)] = predictor
		}
	}

	//boolFlags, nonBoolFlags := boolAndNonBoolFlags(node.Flags)
	pps, err := positionalPredictors(node.Positional, opts.predictors)
	if err != nil {
		return nil, err
	}
	/*cmd.Args = &PositionalPredictor{
		Predictors: pps,
		ArgFlags:   flagNamesWithHyphens(nonBoolFlags...),
		BoolFlags:  flagNamesWithHyphens(boolFlags...),
	}*/
	if len(pps) > 0 {
		cmd.Args = pps[0]
	}

	return &cmd, nil
}

func flagNamesWithHyphens(flags ...*kong.Flag) []string {
	names := make([]string, 0, len(flags)*2)
	if flags == nil {
		return names
	}
	for _, flag := range flags {
		names = append(names, "-"+flag.Name)
		if flag.Short != 0 {
			names = append(names, "-"+string(flag.Short))
		}
	}
	return names
}

// boolAndNonBoolFlags divides a list of flags into boolean and non-boolean flags
func boolAndNonBoolFlags(flags []*kong.Flag) (boolFlags, nonBoolFlags []*kong.Flag) {
	boolFlags = make([]*kong.Flag, 0, len(flags))
	nonBoolFlags = make([]*kong.Flag, 0, len(flags))
	for _, flag := range flags {
		switch flag.Value.IsBool() {
		case true:
			boolFlags = append(boolFlags, flag)
		case false:
			nonBoolFlags = append(nonBoolFlags, flag)
		}
	}
	return boolFlags, nonBoolFlags
}

// kongTag interface for *kong.kongTag
type kongTag interface {
	Has(string) bool
	Get(string) string
}

func tagPredictor(tag kongTag, predictors map[string]complete.Predictor) (complete.Predictor, error) {
	if tag == nil {
		return nil, nil
	}
	if !tag.Has(predictorTag) {
		return nil, nil
	}
	if predictors == nil {
		predictors = map[string]complete.Predictor{}
	}
	predictorName := tag.Get(predictorTag)
	predictor, ok := predictors[predictorName]
	if !ok {
		return nil, fmt.Errorf("no predictor with name %q", predictorName)
	}
	return predictor, nil
}

func valuePredictor(value *kong.Value, predictors map[string]complete.Predictor) (complete.Predictor, error) {
	if value == nil {
		return nil, nil
	}
	predictor, err := tagPredictor(value.Tag, predictors)
	if err != nil {
		return nil, err
	}
	if predictor != nil {
		return predictor, nil
	}
	switch {
	case value.IsBool():
		return predict.Nothing, nil
	case value.Enum != "":
		enumVals := make([]string, 0, len(value.EnumMap()))
		for enumVal := range value.EnumMap() {
			enumVals = append(enumVals, enumVal)
		}
		return predict.Set(enumVals), nil
	default:
		return predict.Something, nil
	}
}

func positionalPredictors(args []*kong.Positional, predictors map[string]complete.Predictor) ([]complete.Predictor, error) {
	res := make([]complete.Predictor, len(args))
	var err error
	for i, arg := range args {
		res[i], err = valuePredictor(arg, predictors)
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

func flagPredictor(flag *kong.Flag, predictors map[string]complete.Predictor) (complete.Predictor, error) {
	return valuePredictor(flag.Value, predictors)
}

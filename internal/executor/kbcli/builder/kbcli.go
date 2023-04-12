package builder

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kubeshop/botkube/internal/executor/kbcli/command"
	"github.com/kubeshop/botkube/pkg/api"
)

var (
	errUnsupportedCommand  = errors.New("unsupported command")
	errRequiredCmdDropdown = errors.New("command dropdown select cannot be empty")
)

const (
	interactiveBuilderIndicator      = "@builder"
	cmdsDropdownCommand              = "@builder --cmds"
	verbsDropdownCommand             = "@builder --verbs"
	resourceNamesDropdownCommand     = "@builder --resource-name"
	resourceNamespaceDropdownCommand = "@builder --namespace"
	filterPlaintextInputCommand      = "@builder --filter-query"
	kbcliCommandName                 = "kbcli"
	dropdownItemsLimit               = 100
	kbcliMissingCommandMsg           = "Please specify the kbcli command"
)

// Kbcli provides functionality to handle interactive kbcli command selection.
type Kbcli struct {
	commandGuard     CommandGuard
	namespaceLister  NamespaceLister
	log              logrus.FieldLogger
	kcRunner         KubectlRunner
	cfg              Config
	defaultNamespace string
	authCheck        AuthChecker
}

// NewKbcli returns a new Kbcli instance.
func NewKbcli(kcRunner KubectlRunner, cfg Config, logger logrus.FieldLogger, guard CommandGuard, defaultNamespace string, lister NamespaceLister, authCheck AuthChecker) *Kbcli {
	return &Kbcli{
		kcRunner:         kcRunner,
		log:              logger,
		namespaceLister:  lister,
		authCheck:        authCheck,
		commandGuard:     guard,
		cfg:              cfg,
		defaultNamespace: defaultNamespace,
	}
}

// ShouldHandle returns true if it's a valid command for interactive builder.
func ShouldHandle(cmd string) bool {
	if cmd == "" || strings.HasPrefix(cmd, interactiveBuilderIndicator) {
		return true
	}
	return false
}

// Handle constructs the interactive command builder messages.
func (e *Kbcli) Handle(ctx context.Context, cmd string, isInteractivitySupported bool, state *slack.BlockActionStates) (api.Message, error) {
	var empty api.Message

	if !isInteractivitySupported {
		e.log.Debug("Interactive kbcli command builder is not supported. Requesting a full kbcli command.")
		return e.message(kbcliMissingCommandMsg)
	}

	allCmds, allVerbs := e.cfg.Allowed.Cmds, e.cfg.Allowed.Verbs
	allCmds = e.commandGuard.FilterSupportedCmds(allCmds)

	if len(allCmds) == 0 {
		msg := fmt.Sprintf("Unfortunately non of configured %q verbs are supported by interactive command builder.", strings.Join(e.cfg.Allowed.Verbs, ","))
		return e.message(msg)
	}
	args := strings.Fields(cmd)
	if len(args) < 2 { // return initial command builder message as there is no builder params
		return e.initialMessage(allCmds)
	}
	cmd = fmt.Sprintf("%s %s", args[0], args[1])

	stateDetails := e.extractStateDetails(state)
	if stateDetails.namespace == "" {
		stateDetails.namespace = e.defaultNamespace
	}

	e.log.WithFields(logrus.Fields{
		"namespace":    stateDetails.namespace,
		"resourceName": stateDetails.resourceName,
		"verb":         stateDetails.verb,
		"cmd":          stateDetails.cmd,
	}).Debug("Extracted Slack state")

	cmds := executorsRunner{
		cmdsDropdownCommand: func() (api.Message, error) {
			return e.renderMessage(ctx, stateDetails, allCmds, allVerbs)
		},
		verbsDropdownCommand: func() (api.Message, error) {
			// the resource type was selected, so clear resource name from command preview.
			stateDetails.resourceName = ""
			e.log.Info("Selecting resource type")
			return e.renderMessage(ctx, stateDetails, allCmds, allVerbs)
		},
		resourceNamesDropdownCommand: func() (api.Message, error) {
			// this is called only when the resource name is directly selected from dropdown, so we need to include
			// it in command preview.
			return e.renderMessage(ctx, stateDetails, allCmds, allVerbs)
		},
		resourceNamespaceDropdownCommand: func() (api.Message, error) {
			// when the namespace was changed, there is a small chance that resource name will be still matching,
			// we will need to do the external call to check that. For now, we clear resource name from command preview.
			stateDetails.resourceName = ""
			return e.renderMessage(ctx, stateDetails, allCmds, allVerbs)
		},
		filterPlaintextInputCommand: func() (api.Message, error) {
			return e.renderMessage(ctx, stateDetails, allCmds, allVerbs)
		},
	}

	msg, err := cmds.SelectAndRun(cmd)
	switch err {
	case nil:
	case command.ErrCmdNotSupported:
		return errMessage(allCmds, ":exclamation: Unfortunately, interactive command builder doesn't support %q command yet.", stateDetails.cmd)
	default:
		e.log.WithField("error", err.Error()).Error("Cannot render the kbcli command builder.")
		return empty, err
	}
	return msg, nil
}

func (e *Kbcli) initialMessage(allCmds []string) (api.Message, error) {
	var empty api.Message

	// We start a new interactive block, so we generate unique ID.
	// Later when we update this message with a new "body" e.g. update command preview
	// the block state remains the same as Slack always see it under the same id.
	// If we use different ID each time we update the message, Slack will clean up the state
	// meaning we will lose information about cmd/verb/resourceName that were previously selected.
	id, err := uuid.NewRandom()
	if err != nil {
		return empty, err
	}
	allCmdsSelect := CmdSelect(allCmds, "")
	if allCmdsSelect == nil {
		return empty, errRequiredCmdDropdown
	}

	msg := KbcliCmdBuilderMessage(id.String(), *allCmdsSelect)
	// we are the initial message, don't replace the original one as we need to send a brand-new message visible only to the user
	// otherwise we can replace a message that is publicly visible.
	msg.ReplaceOriginal = false

	return msg, nil
}

func errMessage(allVerbs []string, errMsgFormat string, args ...any) (api.Message, error) {
	dropdownsBlockID, err := uuid.NewRandom()
	if err != nil {
		return api.Message{}, err
	}

	selects := api.Section{
		Selects: api.Selects{
			ID: dropdownsBlockID.String(),
		},
	}

	allVerbsSelect := CmdSelect(allVerbs, "")
	if allVerbsSelect != nil {
		selects.Selects.Items = []api.Select{
			*allVerbsSelect,
		}
	}

	errBody := api.Section{
		Base: api.Base{
			Body: api.Body{
				Plaintext: fmt.Sprintf(errMsgFormat, args...),
			},
		},
	}

	return api.Message{
		ReplaceOriginal:   true,
		OnlyVisibleForYou: true,
		Sections: []api.Section{
			selects,
			errBody,
		},
	}, nil
}

func (e *Kbcli) renderMessage(ctx context.Context, stateDetails stateDetails, allCmds, allVerbs []string) (api.Message, error) {
	var empty api.Message

	allCmdsSelect := CmdSelect(allCmds, stateDetails.cmd)
	if allCmdsSelect == nil {
		return empty, errRequiredCmdDropdown
	}

	// 1. Refresh verbs list
	matchingVerbs, err := e.getAllowedVerbsSelectList(stateDetails.cmd, allVerbs, stateDetails.verb)
	if err != nil {
		return empty, err
	}

	// 2. If a given command doesn't have assigned verbs,
	//    render:
	//      1. Dropdown with all cmds
	//      2. Filter input
	//      3. Command preview. For example:
	//         kbcli cluster
	if matchingVerbs == nil {
		// we must zero those fields as they are known only if we know the resource type and this cmd doesn't have one :)
		stateDetails.verb = ""
		stateDetails.resourceName = ""
		stateDetails.namespace = ""
		preview := e.buildCommandPreview(stateDetails)
		return KbcliCmdBuilderMessage(
			stateDetails.dropdownsBlockID, *allCmdsSelect,
			WithAdditionalSections(preview...),
		), nil
	}

	// 3. If verb is not on the list anymore,
	//    render:
	//      1. Dropdown with all verbs
	//      2. Dropdown with all related resource types
	//    because we don't know the resource type we cannot render:
	//      1. Resource names - obvious :).
	//      2. Namespaces as we don't know if it's cluster or namespace scoped resource.
	if !e.contains(matchingVerbs, stateDetails.verb) {
		return KbcliCmdBuilderMessage(
			stateDetails.dropdownsBlockID, *allCmdsSelect,
			WithAdditionalSelects(matchingVerbs),
		), nil
	}

	// At this stage we know that:
	//   1. Cmd requires verbs
	//   2. Selected verb is still valid for the selected cmd
	var (
		resNames = e.tryToGetResourceNamesSelect(ctx, stateDetails)
		nsNames  = e.tryToGetNamespaceSelect(ctx, stateDetails)
	)

	if resNames == nil {
		// we must zero those fields as they are known only if we know the resource type and this cmd doesn't have one :)
		stateDetails.resourceName = ""
		stateDetails.namespace = ""
		preview := e.buildCommandPreview(stateDetails)
		return KbcliCmdBuilderMessage(
			stateDetails.dropdownsBlockID, *allCmdsSelect,
			WithAdditionalSelects(matchingVerbs),
			WithAdditionalSections(preview...),
		), nil
	}

	// 4. If a given resource name is not on the list anymore, clear it.
	if !e.contains(resNames, stateDetails.resourceName) {
		stateDetails.resourceName = ""
	}

	// 5. If a given namespace is not on the list anymore, clear it.
	if !e.contains(nsNames, stateDetails.namespace) {
		stateDetails.namespace = ""
	}

	// 6. Render all dropdowns and full command preview.
	preview := e.buildCommandPreview(stateDetails)
	return KbcliCmdBuilderMessage(
		stateDetails.dropdownsBlockID, *allCmdsSelect,
		WithAdditionalSelects(matchingVerbs, resNames, nsNames),
		WithAdditionalSections(preview...),
	), nil
}

func (e *Kbcli) tryToGetResourceNamesSelect(ctx context.Context, state stateDetails) *api.Select {
	e.log.Info("Get resource names")
	if state.verb == "" {
		e.log.Info("Return empty resource name")
		return EmptyResourceNameDropdown()
	}

	// get resource type for the given cmd
	resourceType := e.commandGuard.GetResourceTypeForCmd(state.cmd)
	if resourceType == "" {
		return nil
	}

	cmd := fmt.Sprintf(`get %s --ignore-not-found=true -o go-template='{{range .items}}{{.metadata.name}}{{"\n"}}{{end}}'`, resourceType)
	if state.namespace != "" {
		cmd = fmt.Sprintf("%s -n %s", cmd, state.namespace)
	}
	e.log.Infof("Run cmd %q", cmd)

	out, err := e.kcRunner.RunKubectlCommand(ctx, os.Getenv("KUBECONFIG"), e.defaultNamespace, cmd)
	if err != nil {
		e.log.WithField("error", err.Error()).Error("Cannot fetch resource names. Returning empty resource name dropdown.")
		return EmptyResourceNameDropdown()
	}

	lines := getNonEmptyLines(out)
	if len(lines) == 0 {
		return EmptyResourceNameDropdown()
	}

	return ResourceNamesSelect(overflowSentence(lines), state.resourceName)
}

func (e *Kbcli) tryToGetNamespaceSelect(ctx context.Context, details stateDetails) *api.Select {
	initialNamespace := newDropdownItem(details.namespace, details.namespace)
	initialNamespace = e.appendNamespaceSuffixIfDefault(initialNamespace)

	allNs := []dropdownItem{
		initialNamespace,
	}
	for _, name := range e.collectAdditionalNamespaces(ctx) {
		kv := newDropdownItem(name, name)
		if name == details.namespace {
			// already added, skip it
			continue
		}
		allNs = append(allNs, kv)
	}

	return ResourceNamespaceSelect(allNs, initialNamespace)
}

func (e *Kbcli) collectAdditionalNamespaces(ctx context.Context) []string {
	// if preconfigured, use specified those Namespaces
	if len(e.cfg.Allowed.Namespaces) > 0 {
		return e.cfg.Allowed.Namespaces
	}

	// user didn't narrow down the namespace dropdown, so let's try to get all namespaces.
	allClusterNamespaces, err := e.namespaceLister.List(ctx, metav1.ListOptions{
		Limit: dropdownItemsLimit,
	})
	if err != nil {
		e.log.WithField("error", err.Error()).Error("Cannot fetch all available Kubernetes namespaces, ignoring namespace dropdown...")
		// we cannot fetch other namespaces, so let's render only the default one.
		return nil
	}

	var out []string
	for _, item := range allClusterNamespaces.Items {
		out = append(out, item.Name)
	}

	return out
}

// UX requirement to append the (namespace) suffix if the namespace is called `default`.
func (e *Kbcli) appendNamespaceSuffixIfDefault(in dropdownItem) dropdownItem {
	if in.Name == "default" {
		in.Name += " (namespace)"
	}
	return in
}

// getAllowedVerbsSelectList returns dropdown select with allowed verbs for a given cmd.
func (e *Kbcli) getAllowedVerbsSelectList(cmd string, verbs []string, verb string) (*api.Select, error) {
	allowedVerbs, err := e.commandGuard.GetAllowedVerbsForCmd(cmd, verbs)
	if err != nil {
		return nil, err
	}

	allowedVerbsList := make([]string, 0, len(allowedVerbs))
	for _, item := range allowedVerbs {
		allowedVerbsList = append(allowedVerbsList, item)
	}

	return VerbSelect(allowedVerbsList, verb), nil
}

type stateDetails struct {
	dropdownsBlockID string

	cmd          string
	namespace    string
	verb         string
	resourceName string
	filter       string
}

func (e *Kbcli) extractStateDetails(state *slack.BlockActionStates) stateDetails {
	if state == nil {
		return stateDetails{}
	}

	details := stateDetails{}
	for blockID, blocks := range state.Values {
		if !strings.Contains(blockID, filterPlaintextInputCommand) {
			details.dropdownsBlockID = blockID
		}
		for id, act := range blocks {
			id = strings.TrimPrefix(id, kbcliCommandName)
			id = strings.TrimSpace(id)
			switch id {
			case cmdsDropdownCommand:
				details.cmd = act.SelectedOption.Value
			case verbsDropdownCommand:
				details.verb = act.SelectedOption.Value
			case resourceNamesDropdownCommand:
				details.resourceName = act.SelectedOption.Value
			case resourceNamespaceDropdownCommand:
				details.namespace = act.SelectedOption.Value
			case filterPlaintextInputCommand:
				details.filter = act.Value
			}
		}
	}
	return details
}

func (e *Kbcli) contains(matchingTypes *api.Select, resourceType string) bool {
	if matchingTypes == nil {
		return false
	}

	if matchingTypes.InitialOption != nil && matchingTypes.InitialOption.Value == resourceType {
		return true
	}

	return false
}

func (e *Kbcli) buildCommandPreview(state stateDetails) []api.Section {
	cmd := fmt.Sprintf("%s %s %s", kbcliCommandName, state.cmd, state.verb)

	resourceNameSeparator := " "
	if state.resourceName != "" {
		cmd = fmt.Sprintf("%s%s%s", cmd, resourceNameSeparator, state.resourceName)
	}

	resourceDetails, err := e.commandGuard.GetResourceDetails(state.cmd)
	if err != nil {
		e.log.WithFields(logrus.Fields{
			"state": state,
			"error": err.Error(),
		}).Error("Cannot get resource details")
		return []api.Section{InternalErrorSection()}
	}

	if resourceDetails.Namespaced && state.namespace != "" {
		cmd = fmt.Sprintf("%s -n %s", cmd, state.namespace)
	}

	if state.filter != "" {
		cmd = fmt.Sprintf("%s --filter=%q", cmd, state.filter)
	}

	return PreviewSection(cmd, FilterSection())
}

func (e *Kbcli) message(msg string) (api.Message, error) {
	return api.NewPlaintextMessage(msg, true), nil
}

type (
	executorFunc    func() (api.Message, error)
	executorsRunner map[string]executorFunc
)

func (cmds executorsRunner) SelectAndRun(cmd string) (api.Message, error) {
	cmd = strings.ToLower(cmd)
	fn, found := cmds[cmd]
	if !found {
		return api.Message{}, errUnsupportedCommand
	}
	return fn()
}

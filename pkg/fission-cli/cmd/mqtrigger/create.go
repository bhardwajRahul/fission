/*
Copyright 2019 The Fission Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mqtrigger

import (
	"fmt"

	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fv1 "github.com/fission/fission/pkg/apis/core/v1"
	"github.com/fission/fission/pkg/fission-cli/cliwrapper/cli"
	"github.com/fission/fission/pkg/fission-cli/cmd"
	"github.com/fission/fission/pkg/fission-cli/cmd/spec"
	"github.com/fission/fission/pkg/fission-cli/console"
	flagkey "github.com/fission/fission/pkg/fission-cli/flag/key"
	"github.com/fission/fission/pkg/fission-cli/util"
	"github.com/fission/fission/pkg/mqtrigger/validator"
	"github.com/fission/fission/pkg/utils/uuid"
)

type CreateSubCommand struct {
	cmd.CommandActioner
	trigger *fv1.MessageQueueTrigger
}

func Create(input cli.Input) error {
	return (&CreateSubCommand{}).do(input)
}

func (opts *CreateSubCommand) do(input cli.Input) error {
	err := opts.complete(input)
	if err != nil {
		return err
	}
	return opts.run(input)
}

func (opts *CreateSubCommand) complete(input cli.Input) error {
	mqtName := input.String(flagkey.MqtName)
	if len(mqtName) == 0 {
		console.Warn(fmt.Sprintf("--%v will be soon marked as required flag, see 'help' for details", flagkey.MqtName))
		mqtName = uuid.NewString()
	}
	fnName := input.String(flagkey.MqtFnName)

	userProvidedNS, fnNamespace, err := opts.GetResourceNamespace(input, flagkey.NamespaceFunction)
	if err != nil {
		return fmt.Errorf("error in deleting function : %w", err)
	}

	mqtKind := input.String(flagkey.MqtKind)

	mqType := (fv1.MessageQueueType)(input.String(flagkey.MqtMQType))
	if !validator.IsValidMessageQueue((string)(mqType), mqtKind) {
		return errors.New("unsupported message queue type")
	}

	topic := input.String(flagkey.MqtTopic)
	if len(topic) == 0 {
		return errors.New("topic cannot be empty")
	}

	respTopic := input.String(flagkey.MqtRespTopic)
	if topic == respTopic {
		// TODO maybe this should just be a warning, perhaps
		// allow it behind a --force flag
		return errors.New("listen topic should not equal to response topic")
	}

	errorTopic := input.String(flagkey.MqtErrorTopic)
	maxRetries := input.Int(flagkey.MqtMaxRetries)

	if maxRetries < 0 {
		return errors.New("maximum number of retries must be greater than or equal to 0")
	}

	contentType := input.String(flagkey.MqtMsgContentType)
	if len(contentType) == 0 {
		contentType = "application/json"
	}

	err = checkMQTopicAvailability(mqType, mqtKind, topic, respTopic)
	if err != nil {
		return err
	}

	pollingInterval := int32(input.Int(flagkey.MqtPollingInterval))
	if pollingInterval < 0 {
		return errors.New("polling interval must be greater than or equal to 0")
	}

	cooldownPeriod := int32(input.Int(flagkey.MqtCooldownPeriod))
	if cooldownPeriod < 0 {
		return errors.New("cooldownPeriod interval is the period to wait after the last trigger reported active before scaling the deployment back to 0, it must be greater than or equal to 0")
	}

	minReplicaCount := int32(input.Int(flagkey.MqtMinReplicaCount))
	if minReplicaCount < 0 {
		return errors.New("minReplicaCount must be greater than or equal to 0")
	}

	maxReplicaCount := int32(input.Int(flagkey.MqtMaxReplicaCount))
	if maxReplicaCount < 0 {
		return errors.New("maxReplicaCount must be greater than or equal to 0")
	}

	metadata := make(map[string]string)
	metadataParams := input.StringSlice(flagkey.MqtMetadata)
	_ = util.UpdateMapFromStringSlice(&metadata, metadataParams)

	secret := input.String(flagkey.MqtSecret)

	if input.Bool(flagkey.SpecSave) {
		specDir := util.GetSpecDir(input)
		specIgnore := util.GetSpecIgnore(input)
		fr, err := spec.ReadSpecs(specDir, specIgnore, false)
		if err != nil {
			return fmt.Errorf("error reading spec in '%v': %w", specDir, err)
		}

		exists, err := fr.ExistsInSpecs(fv1.Function{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fnName,
				Namespace: userProvidedNS,
			},
		})
		if err != nil {
			return err
		}
		if !exists {
			console.Warn(fmt.Sprintf("MessageQueueTrigger '%v' references unknown Function '%v', please create it before applying spec",
				mqtName, fnName))
		}
	} else {
		err = util.CheckFunctionExistence(input.Context(), opts.Client(), []string{fnName}, fnNamespace)
		if err != nil {
			return err
		}
	}

	m := metav1.ObjectMeta{
		Name:      mqtName,
		Namespace: fnNamespace,
	}

	if input.Bool(flagkey.SpecSave) || input.Bool(flagkey.SpecDry) {
		m = metav1.ObjectMeta{
			Name:      mqtName,
			Namespace: userProvidedNS,
		}
	}
	opts.trigger = &fv1.MessageQueueTrigger{
		ObjectMeta: m,
		Spec: fv1.MessageQueueTriggerSpec{
			FunctionReference: fv1.FunctionReference{
				Type: fv1.FunctionReferenceTypeFunctionName,
				Name: fnName,
			},
			MessageQueueType: mqType,
			Topic:            topic,
			ResponseTopic:    respTopic,
			ErrorTopic:       errorTopic,
			MaxRetries:       maxRetries,
			ContentType:      contentType,
			PollingInterval:  &pollingInterval,
			CooldownPeriod:   &cooldownPeriod,
			MinReplicaCount:  &minReplicaCount,
			MaxReplicaCount:  &maxReplicaCount,
			Metadata:         metadata,
			Secret:           secret,
			MqtKind:          mqtKind,
		},
	}

	return nil
}

func (opts *CreateSubCommand) run(input cli.Input) error {
	// if we're writing a spec, don't call the API
	// save to spec file or display the spec to console
	if input.Bool(flagkey.SpecDry) {
		return spec.SpecDry(*opts.trigger)
	}

	if input.Bool(flagkey.SpecSave) {
		specFile := fmt.Sprintf("mqtrigger-%v.yaml", opts.trigger.ObjectMeta.Name)
		err := spec.SpecSave(*opts.trigger, specFile, false)
		if err != nil {
			return fmt.Errorf("error saving message queue trigger spec: %w", err)
		}
		return nil
	}

	_, err := opts.Client().FissionClientSet.CoreV1().MessageQueueTriggers(opts.trigger.ObjectMeta.Namespace).Create(input.Context(), opts.trigger, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create message queue trigger: %w", err)
	}

	fmt.Printf("trigger '%s' created\n", opts.trigger.ObjectMeta.Name)
	return nil
}

func checkMQTopicAvailability(mqType fv1.MessageQueueType, mqtKind string, topics ...string) error {
	for _, t := range topics {
		if len(t) > 0 && !validator.IsValidTopic((string)(mqType), t, mqtKind) {
			return fmt.Errorf("invalid topic for %s: %s", mqType, t)
		}
	}
	return nil
}

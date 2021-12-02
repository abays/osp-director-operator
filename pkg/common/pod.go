/*
Copyright 2020 Red Hat

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

package common

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// GetAllPodsWithLabel - get all pods from namespace with a specific label
func GetAllPodsWithLabel(r ReconcilerCommon, labelSelectorMap map[string]string, namespace string) (*corev1.PodList, error) {
	labelSelectorString := labels.Set(labelSelectorMap).String()

	podList, err := r.GetKClient().CoreV1().Pods(namespace).List(
		context.TODO(),
		metav1.ListOptions{
			LabelSelector: labelSelectorString,
		},
	)
	if err != nil {
		return podList, err
	}

	return podList, nil
}

// DeletePodsWithLabel - Delete all pods in namespace of the obj matching label selector
func DeletePodsWithLabel(r ReconcilerCommon, obj metav1.Object, labelSelectorMap map[string]string) error {
	err := r.GetClient().DeleteAllOf(
		context.TODO(),
		&corev1.Pod{},
		client.InNamespace(obj.GetNamespace()),
		client.MatchingLabels(labelSelectorMap),
	)
	if err != nil && !k8s_errors.IsNotFound(err) {
		err = fmt.Errorf("Error DeleteAllOf Pod %v", err)
		return err
	}

	return nil
}

// ExecPodCommand - Execute shell command within a pod
func ExecPodCommand(r ReconcilerCommon, pod corev1.Pod, containerName string, command string) (bytes.Buffer, bytes.Buffer, error) {
	req := r.GetKClient().CoreV1().RESTClient().Post().
		Namespace(pod.Namespace).
		Resource("pods").
		Name(pod.Name).
		SubResource("exec").
		Param("container", containerName).
		Param("stdin", "true").
		Param("stdout", "true").
		Param("stderr", "true").
		Param("tty", "false").
		Param("command", "sh")

	cfg, err := config.GetConfig()

	if err != nil {
		return bytes.Buffer{}, bytes.Buffer{}, err
	}

	r.GetLogger().Info(fmt.Sprintf("AJB URL: %s", req.URL().String()))

	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		return bytes.Buffer{}, bytes.Buffer{}, err
	}

	var buf bytes.Buffer
	var buf2 bytes.Buffer
	// var args []string

	// argString := strings.Join(command, "\n")
	argString := fmt.Sprintf("%s\n", command)
	r.GetLogger().Info(fmt.Sprintf("AJB STR: %s", argString))
	reader := strings.NewReader(argString)

	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  io.Reader(reader),
		Stdout: &buf,
		Stderr: &buf2,
		Tty:    false,
	})

	return buf, buf2, err
}

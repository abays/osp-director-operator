package openstackbackup

import (
	"fmt"

	"github.com/openstack-k8s-operators/osp-director-operator/pkg/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	// We don't consider the following errors to be an actual problem, but success is not achieved:
	// 1. "Connection refused" (SSHd is not available yet, try again next reconcile)
	// 2. "No route to host" (VM is having connectivity issues, try again next reconcile)
	// 3. "unable to start all nodes" (Pacemaker had an issue, try again next reconcile)
	// 4. "unable to stop all nodes" (Pacemaker had an issue, try again next reconcile)
	pacemakerCommandErrorRetryList = []string{"Connection refused", "No route to host", "unable to start all nodes", "unable to stop all nodes"}

	// We don't consider the following errors to be an actual problem, and we consider this to be a "success":
	// 1. "Permission denied" (cloud-admin doesn't exist, so there nothing we can do)
	// 2. "pcs: command not found" (Pacemaker wasn't even installed, so there's nothing to do)
	pacemakerCommandErrorIgnoreList = []string{"pcs: command not found", "Permission denied"}
)

// ExecPacemakerClusterStart - Add a particular node (named "targetName") to a Pacemaker cluster via
// pcs API available at "targetIP"
func ExecPacemakerClusterStart(r common.ReconcilerCommon, obj metav1.Object, targetIP string, targetName string) (bool, error) {
	return execPacemakerCommand(r, obj, targetIP, fmt.Sprintf("cluster start %s", targetName), pacemakerCommandErrorRetryList, pacemakerCommandErrorIgnoreList)
}

// ExecPacemakerClusterStop - Remove a particular node (named "targetName") from a Pacemaker cluster via
// pcs API available at "targetIP"
func ExecPacemakerClusterStop(r common.ReconcilerCommon, obj metav1.Object, targetIP string, targetName string) (bool, error) {
	return execPacemakerCommand(r, obj, targetIP, fmt.Sprintf("cluster stop %s", targetName), pacemakerCommandErrorRetryList, pacemakerCommandErrorIgnoreList)
}

// execPacemakerCommand - execute a "pcs <X>" command using the control plane OpenStackClient and return success/error
func execPacemakerCommand(r common.ReconcilerCommon, obj metav1.Object, targetIP string, command string, retryList []string, ignoreList []string) (bool, error) {
	// Get the OpenStackClient pod
	osClientPod, _, err := common.GetClientPod(r, obj)

	if err != nil {
		return false, err
	}

	// Attempt Pacemaker command
	pcsCommand := fmt.Sprintf("ssh cloud-admin@%s sudo pcs %s", targetIP, command)
	buf, errBuf, err := common.ExecPodCommand(r, *osClientPod, "openstackclient", pcsCommand)

	r.GetLogger().Info(fmt.Sprintf("\"pcs %s\" command stdout: %s", command, buf.String()))

	if err != nil {
		errStr := errBuf.String()
		r.GetLogger().Info(fmt.Sprintf("\"pcs %s\" command stderr: %s", command, errStr))

		// TODO: May need to tighten-up the logic here
		if common.StringContainsSliceElement(errStr, pacemakerCommandErrorRetryList) {
			// We did not succeed, but we're okay with trying again
			return false, nil
		} else if !common.StringContainsSliceElement(errStr, pacemakerCommandErrorIgnoreList) {
			// We did not succeed and this error is an actual problem
			return false, err
		}
	}

	// We succeeded
	return true, nil
}

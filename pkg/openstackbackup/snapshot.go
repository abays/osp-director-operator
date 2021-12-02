package openstackbackup

import (
	"context"
	"fmt"
	"net"
	"sort"

	ospdirectorv1beta1 "github.com/openstack-k8s-operators/osp-director-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/osp-director-operator/pkg/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	snapshotv1 "kubevirt.io/api/snapshot/v1alpha1"
	virtv1 "kubevirt.io/client-go/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var (
	powerOnPatchBytes  = []byte(fmt.Sprintf(`[{"op": "remove", "path": "/spec/running"}, {"op": "add", "path": "/spec/runStrategy", "value": "%s"}]`, string(virtv1.RunStrategyRerunOnFailure)))
	powerOffPatchBytes = []byte(`[{"op": "remove", "path": "/spec/runStrategy"}, {"op": "add", "path": "/spec/running", "value": false}]`)
)

// EnsureVMSnapshotSave - Save snapshots for all VMs in the namespace
func EnsureVMSnapshotSave(r common.ReconcilerCommon, instance *ospdirectorv1beta1.OpenStackBackupRequest, oldStatus *ospdirectorv1beta1.OpenStackBackupRequestStatus, vmSetList *ospdirectorv1beta1.OpenStackVMSetList) ([]string, error) {
	return ensureVMSnapshots(r, instance, oldStatus, vmSetList, nil, ospdirectorv1beta1.BackupSave)
}

// EnsureVMSnapshotRestore - Restore snapshots for all VMs in the OpenStackBackup
func EnsureVMSnapshotRestore(r common.ReconcilerCommon, instance *ospdirectorv1beta1.OpenStackBackupRequest, oldStatus *ospdirectorv1beta1.OpenStackBackupRequestStatus, backup *ospdirectorv1beta1.OpenStackBackup, mode ospdirectorv1beta1.BackupMode) ([]string, error) {
	return ensureVMSnapshots(r, instance, oldStatus, nil, backup, ospdirectorv1beta1.BackupSave)
}

// ensureVMSnapshots - Save or restore snapshots for all VMs in the namespace (for save) or in an OpenStackBackup (for restore)
func ensureVMSnapshots(r common.ReconcilerCommon, instance *ospdirectorv1beta1.OpenStackBackupRequest, oldStatus *ospdirectorv1beta1.OpenStackBackupRequestStatus, vmSetList *ospdirectorv1beta1.OpenStackVMSetList, backup *ospdirectorv1beta1.OpenStackBackup, mode ospdirectorv1beta1.BackupMode) ([]string, error) {
	// Get all existing snapshots/restores first
	existingList := []metav1.Object{}

	// We need a unique label for all snapshots/restores created for this backup request, so use the back
	// request's name and creation timestamp
	snapshotLabel := fmt.Sprintf("%s-%d", instance.Name, instance.CreationTimestamp.Unix())

	listOpts := []client.ListOption{
		client.InNamespace(instance.Namespace),
		client.MatchingLabels(map[string]string{
			BackupSnapshotNameLabelSelector: snapshotLabel,
		}),
	}

	// Create an existing list of snapshot/restores of metav1.Object interfaces that we can convert later when needed
	if mode == ospdirectorv1beta1.BackupSave {
		snapshotList := &snapshotv1.VirtualMachineSnapshotList{}

		err := r.GetClient().List(context.TODO(), snapshotList, listOpts...)

		if err != nil {
			return nil, err
		}

		for _, snapshot := range snapshotList.Items {
			existingList = append(existingList, snapshot.DeepCopy())
		}
	} else if mode == ospdirectorv1beta1.BackupCleanRestore || mode == ospdirectorv1beta1.BackupRestore {
		snapshotRestoreList := &snapshotv1.VirtualMachineRestoreList{}

		err := r.GetClient().List(context.TODO(), snapshotRestoreList, listOpts...)

		if err != nil {
			return nil, err
		}

		for _, snapshotRestore := range snapshotRestoreList.Items {
			existingList = append(existingList, snapshotRestore.DeepCopy())
		}
	}

	// Get the control plane to check if global fencing is enabled (needed below to know
	// to issue certain commands)
	ctlPlane, _, err := common.GetControlPlane(r, instance)

	if err != nil {
		return nil, err
	}

	allVmsPending := []string{}

	// Need an explicit list of OpenStackVMSets, whether directly passed or taken from an OpenStackBackup
	if vmSetList == nil {
		if backup == nil {
			return nil, fmt.Errorf("unable to find VM list for %s for OpenStackBackupRequest %s", mode, instance.Name)
		}

		vmSetList = &backup.Spec.Crs.OpenStackVMSets
	}

	for _, vmSet := range vmSetList.Items {
		vmsPending := []string{}
		vmsPendingCtlPlaneIps := map[string]string{}
		for vmName, vmData := range vmSet.Status.VMHosts {
			if instance.Status.VMSnapshotNames[vmName] == "" {
				// VM has not been snapshotted/restored yet
				vmsPending = append(vmsPending, vmName)

				// FIXME: Don't hardcode network name?
				ip, _, err := net.ParseCIDR(vmData.IPAddresses["ctlplane"])

				if err != nil {
					return nil, err
				}

				// FIXME: Not every VM will have pcs (Pacemaker) installed, so somehow we have
				// to find the control plane network IPs of the controller VMs and use those for
				// all VMs for all VMSets?  May even need to dig into role data, etc, to
				// determine where Pacemaker might live (and which VMs are part of a cluster, etc)
				vmsPendingCtlPlaneIps[vmName] = ip.String()
			}
		}

		// Sort vmsPending to ensure predictive processing
		sort.Strings(vmsPending)

		allVmsPending = append(allVmsPending, vmsPending...)

		// Only power off one VM at a time per VMSet to snapshot/restore it
		if len(vmsPending) > 0 {
			vmName := vmsPending[0]
			vmIP := vmsPendingCtlPlaneIps[vmName]
			var snapshotName string

			if mode == ospdirectorv1beta1.BackupSave {
				// For saves, the snapshot name will be vmName + snapshotLabel
				snapshotName = fmt.Sprintf("%s-%s", vmName, snapshotLabel)
			} else if mode == ospdirectorv1beta1.BackupCleanRestore || mode == ospdirectorv1beta1.BackupRestore {
				// For restores, we need to get the target snapshot name from the OpenStackBackupSpec
				snapshotName = backup.Spec.VMSnapshotNames[vmName]

				if snapshotName == "" {
					return nil, fmt.Errorf("no snapshot for VM %s was found in OpenStackBackup %s while processing restore in OpenStackBackupRequest %s", vmName, backup.Name, instance.Name)
				}
			}

			// Get the VM in case we need to check its state
			vm := &virtv1.VirtualMachine{}

			err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: vmName, Namespace: instance.Namespace}, vm)

			if err != nil {
				return nil, err
			}

			found := false

			for _, existing := range existingList {
				ready := false

				if snapshot, ok := existing.(*snapshotv1.VirtualMachineSnapshot); ok {
					found = (snapshot.Name == snapshotName)
					ready = (snapshot.Status != nil && snapshot.Status.ReadyToUse != nil && *snapshot.Status.ReadyToUse)
				} else if restore, ok := existing.(*snapshotv1.VirtualMachineRestore); ok {
					found = (restore.Spec.VirtualMachineSnapshotName == snapshotName)
					ready = (restore.Status != nil && restore.Status.Complete != nil && *restore.Status.Complete)
				} else {
					return nil, fmt.Errorf("unable to cast snapshot nor restore from VirtualMachine snapshot/restore metav1.Object object")
				}

				if found {
					if ready {
						// Snapshot snapshot/restore is complete, so:
						// 1. Check if VM is powered-on
						// 2. Power-on VM if it isn't already so
						// 3. Add VM back to cluster via "pcs cluster start <vmName>", if necessary
						// 4. If VM is powered-on and ready (and perhaps re-added to a Pacemaker cluster),
						//    add VM name to OpenstackBackupRequest VM snapshot status map

						if vm.Spec.RunStrategy == nil || *vm.Spec.RunStrategy != virtv1.RunStrategyRerunOnFailure {
							// Make sure the VM is powered-on, since it is done restoring
							err := r.GetClient().Patch(context.TODO(), vm, client.RawPatch(types.JSONPatchType, powerOnPatchBytes))

							if err != nil {
								return nil, err
							}

							r.GetLogger().Info(fmt.Sprintf("Power-on request for VM %s was accepted", vmName))
						} else if vm.Status.Ready {
							success := true

							if ctlPlane.Spec.EnableFencing && vmSet.Spec.VMCount > 2 {
								// We need to add the VM back into the Pacemaker cluster
								if success, err = ExecPacemakerClusterStart(r, instance, vmIP, vmName); err != nil {
									return nil, err
								}
							}

							if success {
								// Save VM-to-VM-snapshot entry in map in request status to indicate that
								// the snapshot was saved/restored
								instance.Status.VMSnapshotNames[vmName] = snapshotName
							}
						}
					}

					// Since we found a VM and acted on it, we're done with this VMSet for this reconcile
					break
				}
			}

			if !found {
				// No snapshot/restore exists for this VM and backup request yet, so...
				// 1. If fencing is enabled, remove VM from Pacemaker cluster
				// 2. Power-off VM if it isn't already so
				// 3. Create a snapshot/restore CR for the VM

				shutdownAndCreateCr := true

				if ctlPlane.Spec.EnableFencing && vmSet.Spec.VMCount > 2 {
					// We need to remove the VM from the Pacemaker cluster
					if shutdownAndCreateCr, err = ExecPacemakerClusterStop(r, instance, vmIP, vmName); err != nil {
						return nil, err
					}
				}

				if shutdownAndCreateCr {
					// The VM is ready to be shutdown and snapshotted/restored, so make sure the VM is powered-off
					// and create the snapshot/restore CR
					if vm.Status.Ready {
						err := r.GetClient().Patch(context.TODO(), vm, client.RawPatch(types.JSONPatchType, powerOffPatchBytes))

						if err != nil {
							return nil, err
						}

						r.GetLogger().Info(fmt.Sprintf("Shutdown request for VM %s was accepted", vmName))
					}

					var crFunc func(r common.ReconcilerCommon, vmName string, snapshotName string, namespace string, label string) error

					if mode == ospdirectorv1beta1.BackupSave {
						crFunc = ensureSnapshot
					} else if mode == ospdirectorv1beta1.BackupCleanRestore || mode == ospdirectorv1beta1.BackupRestore {
						crFunc = ensureSnapshotRestore
					}

					if err := crFunc(r, vmName, snapshotName, instance.Namespace, snapshotLabel); err != nil {
						return nil, err
					}
				}
			}
		}
	}

	return allVmsPending, nil
}

func ensureSnapshot(r common.ReconcilerCommon, vmName string, snapshotName string, namespace string, label string) error {
	// Create the snapshot CR
	snapshot := &snapshotv1.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      snapshotName,
			Namespace: namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.GetClient(), snapshot, func() error {
		labels := snapshot.ObjectMeta.GetLabels()

		if labels == nil {
			labels = map[string]string{}
		}

		labels[BackupSnapshotNameLabelSelector] = label
		snapshot.ObjectMeta.SetLabels(labels)

		apiGroup := "kubevirt.io"

		snapshot.Spec.Source = corev1.TypedLocalObjectReference{
			APIGroup: &apiGroup,
			Kind:     "VirtualMachine",
			Name:     vmName,
		}

		return nil
	})

	if err != nil {
		return err
	}

	if op != controllerutil.OperationResultNone {
		r.GetLogger().Info(fmt.Sprintf("VirtualMachineSnapshot %s CR successfully reconciled - operation: %s", snapshotName, string(op)))
	}

	return nil
}

func ensureSnapshotRestore(r common.ReconcilerCommon, vmName string, snapshotName string, namespace string, label string) error {
	// Create the snapshot restore CR
	snapshotRestore := &snapshotv1.VirtualMachineRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      snapshotName,
			Namespace: namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.GetClient(), snapshotRestore, func() error {
		labels := snapshotRestore.ObjectMeta.GetLabels()

		if labels == nil {
			labels = map[string]string{}
		}

		labels[BackupSnapshotNameLabelSelector] = label
		snapshotRestore.ObjectMeta.SetLabels(labels)

		apiGroup := "kubevirt.io"

		snapshotRestore.Spec.VirtualMachineSnapshotName = snapshotName
		snapshotRestore.Spec.Target = corev1.TypedLocalObjectReference{
			APIGroup: &apiGroup,
			Kind:     "VirtualMachine",
			Name:     vmName,
		}

		return nil
	})

	if err != nil {
		return err
	}

	if op != controllerutil.OperationResultNone {
		r.GetLogger().Info(fmt.Sprintf("VirtualMachineRestore %s CR successfully reconciled - operation: %s", snapshotName, string(op)))
	}

	return nil
}

package v1beta1

import (
	ospdirectorv1beta2 "github.com/openstack-k8s-operators/osp-director-operator/api/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this OpenStackBackupRequest to the Hub version (v1beta2).
func (osbr *OpenStackBackupRequest) ConvertTo(dstRaw conversion.Hub) error {
	// Lint complains if func receiver name is different between functions
	// in the same package for the same type target, so think of "osbr" as
	// if it were "src" (which is an OSBackupRequest v1beta1)
	dst := dstRaw.(*ospdirectorv1beta2.OpenStackBackupRequest)
	dst.Spec.BackupMode = ospdirectorv1beta2.BackupMode(osbr.Spec.Mode)
	dst.Spec.NeededThing = "yes"
	dst.ObjectMeta = osbr.ObjectMeta
	return nil
}

// ConvertFrom converts from the Hub version (v1beta2) to this version.
func (osbr *OpenStackBackupRequest) ConvertFrom(srcRaw conversion.Hub) error {
	// Lint complains if func receiver name is different between functions
	// in the same package for the same type target, so think of "osbr" as
	// if it were "dst" (which is an OSBackupRequest v1beta1)
	src := srcRaw.(*ospdirectorv1beta2.OpenStackBackupRequest)
	osbr.Spec.Mode = BackupMode(src.Spec.BackupMode)
	osbr.ObjectMeta = src.ObjectMeta
	return nil
}

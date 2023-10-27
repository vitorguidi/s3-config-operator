/*
Copyright 2023.

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

package controllers

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"

	awsv1beta1 "example.com/api/v1beta1"
)

const s3SHA256HashAnnotation string = "example.com/api/v1beta1/s3filesha256hash"

// AwsS3SecretReconciler reconciles a AwsS3Secret object
type AwsS3SecretReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=aws.example.com,resources=awss3secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=aws.example.com,resources=awss3secrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=aws.example.com,resources=awss3secrets/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AwsS3Secret object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile

func (r *AwsS3SecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Entered reconcile loop.")

	awsS3Secret := &awsv1beta1.AwsS3Secret{}
	err := r.Get(ctx, req.NamespacedName, awsS3Secret)

	if err != nil {
		if errors.IsNotFound(err) {
			//add finalizer logic to delete configmap
			return ctrl.Result{}, nil
		}
		log.Error(err, fmt.Sprintf("Failed to retrieved : %s", req.NamespacedName))
		return ctrl.Result{}, err
	}

	objectSHA256Hash, err := r.getS3ObjectSHA256Hash(ctx, awsS3Secret)
	if err != nil {
		//is this the proper way to handle not being able to query s3?
		return ctrl.Result{}, err
	}

	oldHash, oldHashExists := awsS3Secret.Annotations[s3SHA256HashAnnotation]

	if !oldHashExists || objectSHA256Hash != oldHash {
		awsS3Secret.Annotations[s3SHA256HashAnnotation] = objectSHA256Hash
		err = r.Client.Update(ctx, awsS3Secret)
		if err != nil {
			log.Error(err, fmt.Sprintf("Could not update AwsS3Secret %s", req.NamespacedName))
			//is this the proper way to handle not being able to handle a failed update?
			return ctrl.Result{}, err
		}
	}

	configMap := &corev1.ConfigMap{}

	desiredConfigMapNamespacedName := &types.NamespacedName{
		Namespace: req.Namespace,
		Name:      awsS3Secret.Spec.SecretName,
	}

	objectContent, err := r.getS3ObjectContent(ctx, awsS3Secret)

	if err != nil {
		//is this the proper way to handle not being able to query s3?
		return ctrl.Result{}, err
	}

	err = r.Client.Get(ctx, *desiredConfigMapNamespacedName, configMap)

	if err != nil && errors.IsNotFound(err) {
		log.Info(fmt.Sprintf("ConfigMap not found: %s", *desiredConfigMapNamespacedName))
		configMap := r.buildConfigMapFromS3Data(req.Namespace, awsS3Secret.Spec.SecretName,
			objectContent, objectSHA256Hash)
		err = r.Client.Create(ctx, configMap)
		if err != nil {
			log.Error(err, "Could not create configmap")
			return ctrl.Result{Requeue: true}, err
		}
		log.Info("Successfully created configmap")
		return ctrl.Result{}, nil
	}

	//if the configmap was created, it necessarily has a hash
	configMapS3FileHash, _ := configMap.Annotations[s3SHA256HashAnnotation]
	if configMapS3FileHash != objectSHA256Hash {
		log.Info("Attempting to update configmap data")
		configMap := r.buildConfigMapFromS3Data(req.Namespace, awsS3Secret.Spec.SecretName,
			objectContent, objectSHA256Hash)
		err := r.Client.Update(ctx, configMap)
		if err != nil {
			log.Error(err, "Could not update configmap with the new s3 data")
			return ctrl.Result{}, err
		}
		log.Info("Successfully updated configmap data")
		return ctrl.Result{}, nil
	}

	log.Info(fmt.Sprintf("Nothing to change for %s", req.NamespacedName))
	return ctrl.Result{}, nil
}

func (r *AwsS3SecretReconciler) buildConfigMapFromS3Data(namespace string, configMapName string, data string, sha256hash string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
			Annotations: map[string]string{
				s3SHA256HashAnnotation: sha256hash,
			},
		},
		Data: map[string]string{
			"data": data,
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *AwsS3SecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	s3ChangeEvents := make(chan event.GenericEvent)
	logger := log.Log
	go func() {
		for true {
			time.Sleep(time.Second * 30)
			s3secrets := &awsv1beta1.AwsS3SecretList{}
			err := r.Client.List(context.TODO(), s3secrets)
			if err != nil {
				logger.Error(err, "Failed to fetch aws s3 list in the poll loop")
				continue
			}
			for _, s3secret := range s3secrets.Items {
				go func(s awsv1beta1.AwsS3Secret) {
					sha256hash := s3secret.Annotations[s3SHA256HashAnnotation]
					s3hash, err := r.getS3ObjectSHA256Hash(context.TODO(), &s3secret)
					if err != nil {
						logger.Error(err,
							fmt.Sprintf(
								"Could not query s3 in poll loop for AWS S3 Secret %s %s",
								s3secret.GetName(),
								s3secret.GetNamespace()))
						return
					}
					if sha256hash != s3hash {
						s3ChangeEvents <- event.GenericEvent{
							Object: &s3secret,
						}
					}
				}(s3secret)
			}
		}
	}()
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1beta1.AwsS3Secret{}).
		Owns(&corev1.ConfigMap{}).
		Watches(&source.Channel{Source: s3ChangeEvents}, &handler.EnqueueRequestForObject{}).
		Complete(r)
}

func (r *AwsS3SecretReconciler) getS3ObjectContent(ctx context.Context, awsS3Secret *awsv1beta1.AwsS3Secret) (string, error) {
	log := log.FromContext(ctx)
	awsCfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Error(err, "Failed to fetch aws credentials.")
		return "", err
	}

	// Create an Amazon S3 service client
	client := s3.NewFromConfig(awsCfg)

	input := &s3.GetObjectInput{
		Bucket: aws.String(awsS3Secret.Spec.S3bucket),
		Key:    aws.String(awsS3Secret.Spec.S3file),
	}

	s3FileContents, err := client.GetObject(context.TODO(), input)

	if err != nil {
		log.Error(err, "Unable to fetch s3 object contents")
		return "", err
	}

	defer s3FileContents.Body.Close()

	objectBytes, err := ioutil.ReadAll(s3FileContents.Body)
	if err != nil {
		log.Error(err, "Unable to read bytes from s3 file")
		return "", err
	}

	objectContent := string(objectBytes)
	fmt.Println("S3 File Content:", objectContent)
	return objectContent, nil
}

func (r *AwsS3SecretReconciler) getS3ObjectSHA256Hash(ctx context.Context, awsS3Secret *awsv1beta1.AwsS3Secret) (string, error) {
	log := log.FromContext(ctx)
	awsCfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Error(err, "Failed to fetch aws credentials.")
		return "", err
	}

	// Create an Amazon S3 service client
	client := s3.NewFromConfig(awsCfg)

	params := &s3.GetObjectAttributesInput{
		Bucket:           aws.String(awsS3Secret.Spec.S3bucket),
		Key:              aws.String(awsS3Secret.Spec.S3file),
		ObjectAttributes: []s3types.ObjectAttributes{s3types.ObjectAttributesChecksum},
	}

	s3FileAttributes, err := client.GetObjectAttributes(context.TODO(), params)

	if err != nil {
		log.Error(err, "Unable to fetch s3 object checksum")
		return "", err
	}

	sha256checksum := s3FileAttributes.Checksum.ChecksumSHA256

	fmt.Println("S3 File SHA256 checksum:", sha256checksum)
	return *sha256checksum, nil
}

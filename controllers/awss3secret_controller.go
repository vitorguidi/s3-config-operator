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
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"

	awsv1beta1 "example.com/api/v1beta1"
)

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
	// TODO(user): your logic here
	awsS3Secret := &awsv1beta1.AwsS3Secret{}
	err := r.Get(ctx, req.NamespacedName, awsS3Secret)

	awsCfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Error(err, "Failed to fetch aws credentials.")
		return ctrl.Result{}, err
	}

	// Create an Amazon S3 service client
	client := s3.NewFromConfig(awsCfg)

	input := &s3.GetObjectInput{
		Bucket: aws.String("secret-operator"),
		Key:    aws.String("lol.txt"),
	}

	s3FileContents, err := client.GetObject(context.TODO(), input)

	if err != nil {
		log.Error(err, "Unable to fetch s3 object contents")
		return ctrl.Result{}, err
	}

	defer s3FileContents.Body.Close()

	objectBytes, err := ioutil.ReadAll(s3FileContents.Body)
	if err != nil {
		log.Error(err, "Unable to read bytes from s3 file")
		return ctrl.Result{}, err
	}

	objectContent := string(objectBytes)
	fmt.Println("S3 File Content:", objectContent)

	configMap := &corev1.ConfigMap{}

	desiredConfigMapNamespacedName := &types.NamespacedName{
		Namespace: req.Namespace,
		Name:      awsS3Secret.Spec.SecretName,
	}

	err = r.Client.Get(ctx, *desiredConfigMapNamespacedName, configMap)

	if err != nil && errors.IsNotFound(err) {
		log.Info(fmt.Sprintf("ConfigMap not found: %s", *desiredConfigMapNamespacedName))
		configMap := r.buildConfigMapFromS3Data(req.Namespace, awsS3Secret.Spec.SecretName, objectContent)
		err = r.Client.Create(ctx, configMap)
		if err != nil {
			log.Error(err, "Could not create configmap")
			return ctrl.Result{Requeue: true}, err
		}
		log.Info("Successfully created configmap")
		return ctrl.Result{Requeue: true}, nil
	}

	currentData := configMap.Data
	if currentData["data"] != objectContent {
		log.Info("Attempting to update configmap data")
		configMap := r.buildConfigMapFromS3Data(req.Namespace, awsS3Secret.Spec.SecretName, objectContent)
		err := r.Client.Update(ctx, configMap)
		if err != nil {
			log.Error(err, "Could not update configmap with the new s3 data")
			return ctrl.Result{}, err
		}
		log.Info("Successfully updated configmap data")
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	log.Info("Nothing to change, requeuing")
	return ctrl.Result{RequeueAfter: time.Second * 5}, nil
}

func (r *AwsS3SecretReconciler) buildConfigMapFromS3Data(namespace string, configMapName string, data string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
		},
		Data: map[string]string{
			"data": data,
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *AwsS3SecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1beta1.AwsS3Secret{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}

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
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	testv1alpha1 "github.com/lzeqian/helloworld-operator/api/v1alpha1"
)

// HelloWorldReconciler reconciles a HelloWorld object
type HelloWorldReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger //日志打印
}

//+kubebuilder:rbac:groups=test.jiaozi.com,resources=helloworlds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=test.jiaozi.com,resources=helloworlds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=test.jiaozi.com,resources=helloworlds/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the HelloWorld object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *HelloWorldReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	fmt.Printf("进入自定义程序\n")
	r.Log.Info("recondile被调用" + req.Namespace + "-" + req.Name)
	// TODO(user): your logic here
	helloworld := &testv1alpha1.HelloWorld{}
	err := r.Client.Get(ctx, req.NamespacedName, helloworld)
	if err != nil {
		//如果是找不到异常 说明这个cr已经被删除了
		if errors.IsNotFound(err) {
			r.Log.Info("crd资源已经被删除")
			//停止loop循环不在订阅事件。
			return ctrl.Result{}, nil
		}
		//返回错误，但是继续监听事件
		return ctrl.Result{}, err
	}
	//找到了cr就可以确认cr下的deploy是否存在
	nginxDeployFound := &appsv1.Deployment{}
	//获取当前namespace下的deploy
	errDeploy := r.Client.Get(ctx, types.NamespacedName{Name: helloworld.Name, Namespace: helloworld.Namespace}, nginxDeployFound)
	if errDeploy != nil {
		//不存在，需要创建
		if errors.IsNotFound(errDeploy) {
			r.Log.Info("不存在deploy，新建deploy")
			//类似于yaml的语法创建ngxindeploy
			nginxDeploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      helloworld.Name,
					Namespace: helloworld.Namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &helloworld.Spec.Size,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"hello_name": helloworld.Name,
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"hello_name": helloworld.Name,
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Image: "nginx",
									Name:  "nginx",
									Ports: []corev1.ContainerPort{{
										ContainerPort: 80,
										Name:          "nginx",
									}},
								},
							},
						},
					},
				},
			}
			controllerutil.SetControllerReference(helloworld, nginxDeploy, r.Scheme)
			if err1 := r.Client.Create(ctx, nginxDeploy); err1 != nil {
				r.Log.Info("不存在deploy，新建deploy失败")
				return ctrl.Result{}, errDeploy
			}
			r.Log.Info("不存在deploy，新建deploy成功")
			return ctrl.Result{Requeue: true}, nil
		} else {
			return ctrl.Result{}, errDeploy
		}
	}
	//如果err是空的说明找到了一个已经存在的deploy,需要判断deploy实际的个数和预期crd上的个数是否一致的
	if *nginxDeployFound.Spec.Replicas != helloworld.Spec.Size {
		r.Log.Info("deploy对应pod数量错误，更新deploy为helloword的size")
		//修改原始的对象的spec.replicas
		nginxDeployFound.Spec.Replicas = &helloworld.Spec.Size
		//更新deploy
		if err = r.Update(ctx, nginxDeployFound); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}
	//更新找到的deploy的pod的数量更新到helloworld的status.nodes上
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(helloworld.Namespace),
		client.MatchingLabels(map[string]string{
			"hello_name": helloworld.Name,
		}),
	}
	if err = r.List(ctx, podList, listOpts...); err != nil {
		return ctrl.Result{}, err
	}

	// 更新pod的实际个数的名字写入到helloworld的status上
	podNames := []string{}
	for pn := range podList.Items {
		podNames = append(podNames, podList.Items[pn].Name)
	}
	if !reflect.DeepEqual(podNames, helloworld.Status.Nodes) {
		helloworld.Status.Nodes = podNames
		r.Log.Info("更新状态多helloword的子status")
		if err := r.Status().Update(ctx, helloworld); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *HelloWorldReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		//watch HelloWorld这个crd作为一监控资源
		For(&testv1alpha1.HelloWorld{}).
		//watch Deployment作为第二个监控资源
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

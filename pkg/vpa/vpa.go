package vpa

import (
	"os"
	"time"

	"github.com/golang/glog"
	autoscaling "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	v1beta2 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1beta2"
	autoscalingv1beta2 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned/typed/autoscaling.k8s.io/v1beta2"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Create makes a vpa for every deployment in the namespace
func Create(namespace string, kubeconfig *string, vpaLabels map[string]string, runonce bool, dryrun bool) {
	glog.V(3).Infof("Using Kubeconfig: %s", *kubeconfig)
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		glog.Fatal(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatal(err.Error())
	}

	vpaClientSet, err := autoscalingv1beta2.NewForConfig(config)
	if err != nil {
		glog.Fatal(err.Error())
	}

	// This will run as a loop if run-once is not specified.
	for {
		//Get the list of deployments
		deployments, err := clientset.ExtensionsV1beta1().Deployments(namespace).List(metav1.ListOptions{})
		if err != nil {
			glog.Fatal(err.Error())
		}
		var deploymentNames []string

		vpaListOptions := metav1.ListOptions{
			LabelSelector: labels.Set(vpaLabels).String(),
		}

		existingVPAs, err := vpaClientSet.VerticalPodAutoscalers(namespace).List(vpaListOptions)
		if err != nil {
			glog.Fatal(err.Error())
		}
		var vpaNames []string

		glog.V(2).Infof("There are %d deployments in the namespace", len(deployments.Items))
		for _, deployment := range deployments.Items {
			deploymentNames = append(deploymentNames, deployment.ObjectMeta.Name)
			glog.V(5).Infof("Found Deployment: %v", deployment.ObjectMeta.Name)
		}

		for _, vpa := range existingVPAs.Items {
			vpaNames = append(vpaNames, vpa.ObjectMeta.Name)
			glog.V(5).Infof("Found existing vpa: %v", vpa.ObjectMeta.Name)
		}

		vpaNeeded := difference(deploymentNames, vpaNames)
		glog.V(3).Infof("Diff deployments, vpas: %v", vpaNeeded)

		if len(vpaNeeded) == 0 {
			glog.Info("All VPAs are in sync.")
		} else if len(vpaNeeded) > 0 {
			for _, vpaName := range vpaNeeded {
				updateMode := v1beta2.UpdateModeOff
				vpa := &v1beta2.VerticalPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						Name:   vpaName,
						Labels: vpaLabels,
					},
					Spec: v1beta2.VerticalPodAutoscalerSpec{
						TargetRef: &autoscaling.CrossVersionObjectReference{
							APIVersion: "extensions/v1beta1",
							Kind:       "Deployment",
							Name:       vpaName,
						},
						UpdatePolicy: &v1beta2.PodUpdatePolicy{
							UpdateMode: &updateMode,
						},
					},
				}

				if !dryrun {
					glog.Infof("Creating vpa: %s", vpaName)
					glog.V(9).Infof("%v", vpa)
					_, err := vpaClientSet.VerticalPodAutoscalers(namespace).Create(vpa)
					if err != nil {
						glog.Errorf("Error creating vpa: %v", err)
					}
				} else {
					glog.Infof("Dry run was set. Not creating vpa: %v", vpaName)
				}
			}
		} else {
			// This should never ever happen
			glog.Fatal("Got a negative number of vpaNeeded")
		}

		if runonce {
			glog.Infof("Exiting due to run-once=true.")
			os.Exit(0)
		}

		// This controls the loop timing.
		time.Sleep(30 * time.Second)
	}
}

func difference(a, b []string) (diff []string) {
	m := make(map[string]bool)

	for _, item := range b {
		m[item] = true
	}

	for _, item := range a {
		if _, ok := m[item]; !ok {
			diff = append(diff, item)
		}
	}
	return
}

// AutoPilotK8s - An autonomous Kubernetes resource manager

package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
)

type AutoPilotController struct {
	clientset *kubernetes.Clientset
	informer  cache.SharedIndexInformer
	queue     workqueue.RateLimitingInterface
}

func NewAutoPilotController(clientset *kubernetes.Clientset, informer cache.SharedIndexInformer) *AutoPilotController {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(key)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			if err == nil {
				queue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(key)
			}
		},
	})

	return &AutoPilotController{
		clientset: clientset,
		informer:  informer,
		queue:     queue,
	}
}

func (c *AutoPilotController) Run(stopCh chan struct{}) {
	defer c.queue.ShutDown()

	fmt.Println("Starting AutoPilot Controller...")
	go c.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		log.Fatalf("Failed to sync cache")
	}

	for {
		key, shutdown := c.queue.Get()
		if shutdown {
			break
		}
		fmt.Printf("Managing Resource: %s\n", key)
		c.queue.Done(key)
	}
}

func main() {
	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating Kubernetes client: %s", err.Error())
	}

	factory := informers.NewSharedInformerFactory(clientset, 10*time.Minute)
	informer := factory.Core().V1().Pods().Informer()

	controller := NewAutoPilotController(clientset, informer)

	stopCh := make(chan struct{})
	defer close(stopCh)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM, syscall.SIGINT)

	go controller.Run(stopCh)

	<-signalChan
	fmt.Println("Shutdown signal received, exiting...")
}

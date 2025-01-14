package service

import (
	"context"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"neutron/internal"
	"sync"
)

type Looter struct {
	Clientset *kubernetes.Clientset
	Namespace string
	Repo      *internal.Repository
}

func NewLooter(namespace string, repo *internal.Repository, configFile string) *Looter {
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", configFile)
	if err != nil {
		log.Fatal(err)
	}
	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		log.Fatal(err)
	}
	return &Looter{
		Clientset: clientset,
		Namespace: namespace,
		Repo:      repo,
	}
}

func (l *Looter) FetchCompletedJobLog() error {
	log.Println("Fetching completed job log...")
	ctx := context.Background()
	jobs, err := l.Clientset.BatchV1().Jobs(l.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	jobChan := make(chan batchv1.Job, len(jobs.Items))
	var wg sync.WaitGroup
	numWorkers := 5
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(looter *Looter) {
			defer wg.Done()
			for job := range jobChan {
				if isJobCompleted(&job) {
					err := looter.processJob(context.Background(), &job)
					if err != nil {
						log.Printf("error to fetch job logs, job_name=%s, err=%v\n", job.Name, err)
					} else {
						log.Printf("fetch job logs, job_name=%s\n", job.Name)
						_ = looter.removeJobs(job.Name)
					}
				}
			}
		}(l)
	}

	for _, job := range jobs.Items {
		jobChan <- job
	}
	close(jobChan)
	wg.Wait()
	return err
}

func (l *Looter) processJob(ctx context.Context, job *batchv1.Job) error {
	labelSelector := metav1.FormatLabelSelector(job.Spec.Selector)
	pods, err := l.Clientset.CoreV1().Pods(l.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			logContent, err := l.fetchJobLogs(ctx, pod)
			if err != nil {
				return err
			}
			err = l.Repo.AddPodLog(job.Name, pod.Name, logContent, string(pod.Status.Phase))
			if err != nil {
				return err
			}
		}
	}
	status := internal.JobStatus{
		WebhookType: job.Annotations["sourceType"],
		RepoUrl:     job.Annotations["gitPath"],
		TriggerType: job.Annotations["triggerType"],
		ProjectUrl:  job.Annotations["sourceLink"],
		Active:      int(job.Status.Active),
		Succeeded:   int(job.Status.Succeeded),
		Failed:      int(job.Status.Failed),
	}
	err = l.Repo.UpdateJobStatus(job.Name, status)
	return err
}

func (l *Looter) fetchJobLogs(ctx context.Context, pod corev1.Pod) (string, error) {
	var logs string
	podLogOpts := &corev1.PodLogOptions{}
	logsRequest := l.Clientset.CoreV1().Pods(l.Namespace).GetLogs(pod.Name, podLogOpts)
	stream, err := logsRequest.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	buf := make([]byte, 32*1024)
	for {
		n, err := stream.Read(buf)
		if err != nil && n == 0 {
			break
		}
		if n > 0 {
			logs += string(buf[:n])
		}
	}
	return logs, nil
}

func (l *Looter) removeJobs(jobName string) error {
	err := l.Clientset.BatchV1().Jobs(l.Namespace).Delete(context.Background(), jobName, metav1.DeleteOptions{})
	return err
}

func isJobCompleted(job *batchv1.Job) bool {
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

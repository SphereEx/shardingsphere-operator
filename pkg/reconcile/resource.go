/*
 * Copyright (c) 2022.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package reconcile

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"html/template"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	shardingspherev1alpha1 "sphere-ex.com/shardingsphere-operator/api/v1alpha1"
	"strconv"
	"strings"
)

func ConstructCascadingDeployment(proxy *shardingspherev1alpha1.Proxy) *appsv1.Deployment {
	dp := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxy.Name,
			Namespace: proxy.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(proxy.GetObjectMeta(), proxy.GroupVersionKind()),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
			Replicas: &proxy.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"apps": proxy.Name,
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"apps": proxy.Name,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:            "proxy",
							Image:           fmt.Sprintf("apache/shardingsphere-proxy:%s", proxy.Spec.Version),
							ImagePullPolicy: v1.PullIfNotPresent,
							Ports: []v1.ContainerPort{
								{
									ContainerPort: proxy.Spec.Port,
								},
							},
							Env: []v1.EnvVar{
								{
									Name:  "PORT",
									Value: strconv.FormatInt(int64(proxy.Spec.Port), 10),
								},
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/opt/shardingsphere-proxy/conf",
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "config",
							VolumeSource: v1.VolumeSource{
								ConfigMap: &v1.ConfigMapVolumeSource{
									LocalObjectReference: v1.LocalObjectReference{
										Name: proxy.Spec.ProxyConfigName,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	return processOptionalParameter(proxy, dp)
}

func ConstructCascadingService(proxy *shardingspherev1alpha1.Proxy) *v1.Service {

	svc := v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxy.Name,
			Namespace: proxy.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(proxy.GetObjectMeta(), proxy.GroupVersionKind()),
			},
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{
				"apps": proxy.Name,
			},
			Type: proxy.Spec.ServiceType.Type,
			Ports: []v1.ServicePort{
				{
					Name: "proxy-port",
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: proxy.Spec.Port,
					},
					Port: proxy.Spec.Port,
				},
			},
		},
	}
	if proxy.Spec.ServiceType.Type == v1.ServiceTypeNodePort {
		svc.Spec.Ports[0].NodePort = proxy.Spec.ServiceType.NodePort
	}
	return &svc
}

func addInitContainer(dp *appsv1.Deployment, mysql *shardingspherev1alpha1.MySQLDriver) {
	scriptStr := strings.Builder{}
	t1, _ := template.New("shell").Parse(`wget https://repo1.maven.org/maven2/mysql/mysql-connector-java/{{ .Version }}/mysql-connector-java-{{ .Version }}.jar;
wget https://repo1.maven.org/maven2/mysql/mysql-connector-java/{{ .Version }}/mysql-connector-java-{{ .Version }}.jar.md5;
if [ $(md5sum /mysql-connector-java-{{ .Version }}.jar | cut -d ' ' -f1) = $(cat /mysql-connector-java-{{ .Version }}.jar.md5) ];
then echo success;
else echo failed;exit 1;fi;mv /mysql-connector-java-{{ .Version }}.jar /opt/shardingsphere-proxy/ext-lib`)
	_ = t1.Execute(&scriptStr, mysql)
	dp.Spec.Template.Spec.InitContainers = []v1.Container{
		{
			Name:    "download-mysql-connect",
			Image:   "busybox:1.35.0",
			Command: []string{"/bin/sh", "-c", scriptStr.String()},
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "mysql-connect-jar",
					MountPath: "/opt/shardingsphere-proxy/ext-lib",
				},
			},
		},
	}
	dp.Spec.Template.Spec.Containers[0].VolumeMounts = append(dp.Spec.Template.Spec.Containers[0].VolumeMounts, v1.VolumeMount{
		Name:      "mysql-connect-jar",
		MountPath: "/opt/shardingsphere-proxy/ext-lib",
	},
	)

	dp.Spec.Template.Spec.Volumes = append(dp.Spec.Template.Spec.Volumes, v1.Volume{
		Name: "mysql-connect-jar",
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		},
	})

}

func updateInitContainer(dp *appsv1.Deployment, mysql *shardingspherev1alpha1.MySQLDriver) {

	if len(dp.Spec.Template.Spec.InitContainers) != 0 {
		scriptStr := strings.Builder{}
		t1, _ := template.New("shell").Parse(`wget https://repo1.maven.org/maven2/mysql/mysql-connector-java/{{ .Version }}/mysql-connector-java-{{ .Version }}.jar;
wget https://repo1.maven.org/maven2/mysql/mysql-connector-java/{{ .Version }}/mysql-connector-java-{{ .Version }}.jar.md5;
if [ $(md5sum /mysql-connector-java-{{ .Version }}.jar | cut -d ' ' -f1) = $(cat /mysql-connector-java-{{ .Version }}.jar.md5) ];
then echo success;
else echo failed;exit 1;fi;mv /mysql-connector-java-{{ .Version }}.jar /opt/shardingsphere-proxy/ext-lib`)
		_ = t1.Execute(&scriptStr, mysql)
		dp.Spec.Template.Spec.InitContainers = []v1.Container{
			{
				Name:    "download-mysql-connect",
				Image:   "busybox:1.35.0",
				Command: []string{"/bin/sh", "-c", scriptStr.String()},
				VolumeMounts: []v1.VolumeMount{
					{
						Name:      "mysql-connect-jar",
						MountPath: "/opt/shardingsphere-proxy/ext-lib",
					},
				},
			},
		}
	} else {
		addInitContainer(dp, mysql)
	}

}

func processOptionalParameter(proxy *shardingspherev1alpha1.Proxy, dp *appsv1.Deployment) *appsv1.Deployment {
	if proxy.Spec.MySQLDriver != nil {
		addInitContainer(dp, proxy.Spec.MySQLDriver)
	}

	//TODO: 更好的实现默认值添加和非默认值赋值
	if proxy.Spec.Resources != nil {
		dp.Spec.Template.Spec.Containers[0].Resources = *proxy.Spec.Resources
	} else {
		cpu, _ := resource.ParseQuantity("0.2")
		memory, _ := resource.ParseQuantity("1.6Gi")
		dp.Spec.Template.Spec.Containers[0].Resources = v1.ResourceRequirements{
			Requests: v1.ResourceList{
				"cpu":    cpu,
				"memory": memory,
			},
		}
	}
	if proxy.Spec.LivenessProbe != nil {
		dp.Spec.Template.Spec.Containers[0].LivenessProbe = proxy.Spec.LivenessProbe
	} else {
		dp.Spec.Template.Spec.Containers[0].LivenessProbe = &v1.Probe{
			ProbeHandler: v1.ProbeHandler{
				TCPSocket: &v1.TCPSocketAction{
					Port: intstr.FromInt(int(proxy.Spec.Port)),
				},
			},

			PeriodSeconds: 10,
		}
	}
	if proxy.Spec.ReadinessProbe != nil {
		dp.Spec.Template.Spec.Containers[0].ReadinessProbe = proxy.Spec.ReadinessProbe
	} else {
		dp.Spec.Template.Spec.Containers[0].ReadinessProbe = &v1.Probe{
			ProbeHandler: v1.ProbeHandler{
				TCPSocket: &v1.TCPSocketAction{
					Port: intstr.FromInt(int(proxy.Spec.Port)),
				},
			},
			PeriodSeconds: 10,
		}
	}
	if proxy.Spec.StartupProbe != nil {
		dp.Spec.Template.Spec.Containers[0].StartupProbe = proxy.Spec.StartupProbe
	} else {
		dp.Spec.Template.Spec.Containers[0].StartupProbe = &v1.Probe{
			ProbeHandler: v1.ProbeHandler{
				TCPSocket: &v1.TCPSocketAction{
					Port: intstr.FromInt(int(proxy.Spec.Port)),
				},
			},
			PeriodSeconds:    5,
			FailureThreshold: 12,
		}
	}
	return dp
}

// ConstructCascadingConfigmap Construct spec resources to Configmap
func ConstructCascadingConfigmap(proxyConfig *shardingspherev1alpha1.ProxyConfig) *v1.ConfigMap {
	y := toYaml(proxyConfig)
	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxyConfig.Name,
			Namespace: proxyConfig.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(proxyConfig.GetObjectMeta(), proxyConfig.GroupVersionKind()),
			},
		},
		Data: map[string]string{
			"server.yaml": y,
		},
	}

}

// ToYaml Convert ProxyConfig spec content to yaml format
func toYaml(proxyConfig *shardingspherev1alpha1.ProxyConfig) string {

	for i := 0; i < len(proxyConfig.Spec.Authority.Users); i++ {
		proxyConfig.Spec.Authority.Users[i].UserConfig = proxyConfig.Spec.Authority.Users[i].UserName +
			"@" + proxyConfig.Spec.Authority.Users[i].HostName +
			":" + proxyConfig.Spec.Authority.Users[i].PassWord
	}
	y, _ := yaml.Marshal(proxyConfig.Spec)
	return string(y)
}

func UpdateDeployment(proxy *shardingspherev1alpha1.Proxy, runtimeDeployment *appsv1.Deployment) {
	runtimeDeployment.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("apache/shardingsphere-proxy:%s", proxy.Spec.Version)
	runtimeDeployment.Spec.Replicas = &proxy.Spec.Replicas
	runtimeDeployment.Spec.Template.Spec.Volumes[0].ConfigMap.Name = proxy.Spec.ProxyConfigName
	runtimeDeployment.Spec.Template.Spec.Containers[0].Env[0].Value = strconv.FormatInt(int64(proxy.Spec.Port), 10)
	runtimeDeployment.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort = proxy.Spec.Port
	if proxy.Spec.MySQLDriver.Version != "" {
		updateInitContainer(runtimeDeployment, proxy.Spec.MySQLDriver)
	}
	runtimeDeployment.Spec.Template.Spec.Containers[0].Resources = *proxy.Spec.Resources
	runtimeDeployment.Spec.Template.Spec.Containers[0].LivenessProbe = proxy.Spec.LivenessProbe
	runtimeDeployment.Spec.Template.Spec.Containers[0].ReadinessProbe = proxy.Spec.ReadinessProbe
	runtimeDeployment.Spec.Template.Spec.Containers[0].StartupProbe = proxy.Spec.StartupProbe

}

// Copyright © 2020 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package client

import (
	"fmt"

	"github.com/banzaicloud/kafka-operator/api/v1beta1"
	"github.com/banzaicloud/kafka-operator/pkg/util/kafka"
)

func UseSSL(cluster *v1beta1.KafkaCluster) bool {
	return cluster.Spec.ListenersConfig.InternalListeners[determineInternalListenerForInnerCom(cluster.Spec.ListenersConfig.InternalListeners)].Type.IsSSL()
}

func determineInternalListenerForInnerCom(internalListeners []v1beta1.InternalListenerConfig) int {
	for id, val := range internalListeners {
		if val.UsedForInnerBrokerCommunication {
			return id
		}
	}
	return 0
}

func GenerateKafkaAddressWithoutPort(cluster *v1beta1.KafkaCluster) string {
	if cluster.Spec.HeadlessServiceEnabled {
		return fmt.Sprintf("%s.%s.svc.%s",
			fmt.Sprintf(kafka.HeadlessServiceTemplate, cluster.Name),
			cluster.Namespace,
			cluster.Spec.GetKubernetesClusterDomain(),
		)
	}
	return fmt.Sprintf("%s.%s.svc.%s",
		fmt.Sprintf(kafka.AllBrokerServiceTemplate, cluster.Name),
		cluster.Namespace,
		cluster.Spec.GetKubernetesClusterDomain(),
	)
}

func GenerateKafkaAddress(cluster *v1beta1.KafkaCluster) string {
	return fmt.Sprintf("%s:%d", GenerateKafkaAddressWithoutPort(cluster),
		cluster.Spec.ListenersConfig.InternalListeners[determineInternalListenerForInnerCom(cluster.Spec.ListenersConfig.InternalListeners)].ContainerPort)
}

/*
 * This file is part of the KubeVirt project
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
 *
 * Copyright 2018 Red Hat, Inc.
 *
 */

package tests_test

import (
	"flag"
	"os/exec"
	"time"
	"encoding/xml"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/api"
	"kubevirt.io/kubevirt/pkg/api/v1"
	"kubevirt.io/kubevirt/pkg/kubecli"
	"kubevirt.io/kubevirt/tests"
)

var _ = Describe("IOThreads", func() {
	flag.Parse()

	virtClient, err := kubecli.GetKubevirtClient()
	tests.PanicOnError(err)

	gracePeriod := int64(30)
	sharedPolicy := v1.IOThreadsPolicyShared
	_true := true
	IOThreadSpec := v1.VirtualMachineInstanceSpec{
	    TerminationGracePeriodSeconds: &gracePeriod,
	    Domain: v1.DomainSpec{
		Resources: v1.ResourceRequirements{
		    Requests: k8sv1.ResourceList{
			k8sv1.ResourceMemory: resource.MustParse("1024M"),
			k8sv1.ResourceCPU: resource.MustParse("2"),
		    },
		},
		IOThreadsPolicy: &sharedPolicy,

		Devices: v1.Devices{
		    Disks: []v1.Disk{
			{
			    Name:       "ded1",
			    VolumeName: "ded1volume",
			    DiskDevice: v1.DiskDevice{
				Disk: &v1.DiskTarget{
				    Bus: "virtio",
				},
			    },
			    DedicatedIOThread: &_true,
			},
			{
			    Name:       "shr1",
			    VolumeName: "shr1volume",
			    DiskDevice: v1.DiskDevice{
				Disk: &v1.DiskTarget{
				    Bus: "virtio",
				},
			    },
			},
			{
			    Name:       "shr2",
			    VolumeName: "shr2volume",
			    DiskDevice: v1.DiskDevice{
				Disk: &v1.DiskTarget{
				    Bus: "virtio",
				},
			    },
			},
		    },
		},
	    },
	    Volumes: []v1.Volume{
		{
		    Name: "ded1volume",
		    VolumeSource: v1.VolumeSource{
			EmptyDisk: &v1.EmptyDiskSource{
			    Capacity: resource.MustParse("1G"),
			},
		    },
		},
		{
		    Name: "shr1volume",
		    VolumeSource: v1.VolumeSource{
			EmptyDisk: &v1.EmptyDiskSource{
			    Capacity: resource.MustParse("1G"),
			},
		    },
		},
		{
		    Name: "shr2volume",
		    VolumeSource: v1.VolumeSource{
			EmptyDisk: &v1.EmptyDiskSource{
			    Capacity: resource.MustParse("1G"),
			},
		    },
		},
	    },
	}

	var IOThreadVMI *v1.VirtualMachineInstance

	BeforeEach(func() {
		tests.BeforeTestCleanup()
		IOThreadVMI = tests.NewRandomVMI()
		IOThreadVMI.Spec = IOThreadSpec
	})

	Context("IOThreads Policies", func() {

		// CNV-1046
		It("Virtual Disk Settings - IOThreads", func() {

			// How many symbols in the names of pod and VMI shoul match
			symbols_to_compare := 30

			IOThreadVMI, err := virtClient.VirtualMachineInstance(tests.NamespaceTestDefault).Create(IOThreadVMI)
			// Step 1
			Expect(err).ToNot(HaveOccurred())
			Expect(len(IOThreadVMI.Name) > symbols_to_compare).To(BeTrue())

			// get VMI from the pod
			listOptions := metav1.ListOptions{}
			podList, err := virtClient.CoreV1().Pods(tests.NamespaceTestDefault).List(listOptions)
			// Step 2
			Expect(err).ToNot(HaveOccurred())
			Expect(podList.Items).To(HaveLen(1))
			Expect(podList.Items[0].Name).To(HavePrefix("virt-launcher-" + IOThreadVMI.Name[:symbols_to_compare]))

			getOptions := metav1.GetOptions{}
			resultVMI, err := virtClient.VirtualMachineInstance(tests.NamespaceTestDefault).Get(IOThreadVMI.Name, &getOptions)
			// Step 3
			Expect(err).ToNot(HaveOccurred())

			// Step 4.1
			Expect(*resultVMI.Spec.Domain.IOThreadsPolicy).To(Equal(sharedPolicy))
			// Step 4.2
			ded1_present := false
			for _, disk := range resultVMI.Spec.Domain.Devices.Disks {
				if disk.Name == "ded1" {
					ded1_present = true
					Expect(*disk.DedicatedIOThread).To(BeTrue())
				}
			}
			Expect(ded1_present).To(BeTrue())

			// Step 5 is NA for automatic testing

			duration := time.Duration(60)*time.Second
			time.Sleep(duration)
			//tests.WaitUntilVMIReadyWithNamespace(tests.NamespaceTestDefault, resultVMI, tests.LoggedInCirrosExpecter)
			command := "/usr/local/bin/oc project kubevirt-test-default && "
			command += "/usr/local/bin/kubectl"
			command += " exec " + podList.Items[0].Name
			command += " --container compute cat"
			command += " /var/run/libvirt/qemu/kubevirt-test-default_" + IOThreadVMI.Name + ".xml"
			output, err := exec.Command("/bin/bash", "-c", command).Output()
			type DomStatus struct {
				Domain api.DomainSpec	`xml:"domain"`
			}
			domStat := &DomStatus{}
			err = xml.Unmarshal(output, domStat)

			// Step 6
			Expect(err).ToNot(HaveOccurred())
			ded1_present = false
			shr_num := 0
			for _, disk := range domStat.Domain.Devices.Disks {
				if disk.Alias.Name == "ded1" {
					ded1_present = true
					Expect(int(*disk.Driver.IOThread)).To(Equal(2))
				}
				if strings.HasPrefix(disk.Alias.Name, "shr") {
					shr_num += 1
					Expect(int(*disk.Driver.IOThread)).To(Equal(1))
				}
			}
			Expect(ded1_present).To(BeTrue())
			Expect(shr_num).To(Equal(2))
			Expect(domStat.Domain.IOThreads.IOThreads == 2).To(BeTrue())
		})

	})
})


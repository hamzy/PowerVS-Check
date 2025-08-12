# PowerVS-Check
Useful tool to check OpenShift clusters created on IBM Cloud PowerVS

CLI opitons:
- [check-ci](https://github.com/hamzy/PowerVS-Check#check-ci)
- [check-capi-kubeconfig](https://github.com/hamzy/PowerVS-Check#check-capi-kubeconfig)
- [check-create](https://github.com/hamzy/PowerVS-Check#check-create)
- [check-kubeconfig](https://github.com/hamzy/PowerVS-Check#check-kubeconfig)
- [create-jumpbox](https://github.com/hamzy/PowerVS-Check#create-jumpbox)

## check-ci

This is for checking existing CI objects.

Example usage:

`$ PowerVS-Check-Create check-ci --apiKey ${IBMCLOUD_API_KEY} -metadata ci-zone02.json`

args:
- `apiKey`your IBM Cloud API key

- `metadata` location of a special json file containing the followig:

```
{
  "region": "",
  "vpcRegion": "",
  "zone": "",
  "resourceGroup": "",
  "serviceInstance": "",
  "vpc": "",
  "transitGateway": ""
}
```

- `shouldClean` defaults to `false`

- `shouldDebug` defauts to `false`

## check-capi-kubeconfig

This is for checking the progress of an ongoing `create cluster` operation of the OpenShift IPI installer.  Run this in another window while the installer deploys a cluster.  This is for the first part of a CAPI installation.

Example usage:

`$ PowerVS-Check-Create check-capi-kubeconfig -kubeconfig  ./ocp-test/.clusterapi_output/envtest.kubeconfig`

args:
- `kubeconfig` the location of the CAPI kubeconfig file

- `shouldDebug` defauts to `false`

## check-create

This is for checking the progress of an ongoing `create cluster` operation of the OpenShift IPI installer.  Run this in another window while the installer deploys a cluster.  This is for the second part of a CAPI installation.

Example usage:

`$ PowerVS-Check-Create check-create --apiKey ${IBMCLOUD_API_KEY} -metadata ./ocp-test/metadata.json`

args:
- `apiKey`your IBM Cloud API key

- `metadata` location of the json file which the `openshift-install` program created:

- `shouldDebug` defauts to `false`

## check-kubeconfig

This is for checking the progress of an ongoing `create cluster` operation of the OpenShift IPI installer.  Run this in another window while the installer deploys a cluster.  This is for the second part of a CAPI installation.

Example usage:

`$ PowerVS-Check-Create check-kubeconfig -kubeconfig  ./ocp-test-mad02/auth/kubeconfig`

args:
- `kubeconfig` the location of the IPI kubeconfig file

- `shouldDebug` defauts to `false`

## create-jumpbox

This is used to create an accessible VM that can be used to then connect to the OpenShift cluster's bootstrap, master, and worker VMs.

Example usage:

`$ PowerVS-Check-Create create-jumpbox --apiKey ${IBMCLOUD_API_KEY} -metadata ./ocp-test/metadata.json -imageName ibm-centos-stream-10-amd64-3 -keyName hamzy-key -shouldDebug true`

args:
- `apiKey`your IBM Cloud API key

- `metadata` location of the json file which the `openshift-install` program created:

- `imageName` is the name of a bootable image which the VM uses.  To find out the options, do not specify this argument when running the program.

- `keyName` is the name of your ssh key that has been created in the IBM Cloud.

- `shouldDebug` defauts to `false`

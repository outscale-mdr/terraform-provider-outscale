package outscale

import (
	"context"
	"fmt"
	oscgo "github.com/outscale/osc-sdk-go/v2"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

func resourceOutscaleOAPIVirtualGateway() *schema.Resource {
	return &schema.Resource{
		Create: resourceOutscaleOAPIVirtualGatewayCreate,
		Read:   resourceOutscaleOAPIVirtualGatewayRead,
		Update: resourceOutscaleOAPIVirtualGatewayUpdate,
		Delete: resourceOutscaleOAPIVirtualGatewayDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"connection_type": {
				Type:     schema.TypeString,
				Required: true,
			},

			"net_to_virtual_gateway_links": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"state": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"net_id": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
			"state": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"virtual_gateway_id": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"request_id": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"tags": tagsListOAPISchema(),
		},
	}
}

func resourceOutscaleOAPIVirtualGatewayCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).OSCAPI
	connectType, connecTypeOk := d.GetOk("connection_type")
	createOpts := oscgo.CreateVirtualGatewayRequest{}
	if connecTypeOk {
		createOpts.SetConnectionType(connectType.(string))
	}

	var resp oscgo.CreateVirtualGatewayResponse
	var err error

	err = resource.Retry(5*time.Minute, func() *resource.RetryError {
		resp, _, err = conn.VirtualGatewayApi.CreateVirtualGateway(context.Background()).CreateVirtualGatewayRequest(createOpts).Execute()
		if err != nil {
			if strings.Contains(err.Error(), "RequestLimitExceeded:") {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		return resource.NonRetryableError(err)
	})
	if err != nil {
		return fmt.Errorf("Error creating VPN gateway: %s", err)
	}

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"pending", "ending/wait"},
		Target:     []string{"available"},
		Refresh:    virtualGatewayStateRefreshFunc(conn, resp.VirtualGateway.GetVirtualGatewayId(), "terminated"),
		Timeout:    d.Timeout(schema.TimeoutCreate),
		Delay:      10 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	_, err = stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf(
			"Error waiting for instance (%s) to become created: %s", d.Id(), err)
	}

	virtualGateway := resp.GetVirtualGateway()
	d.SetId(virtualGateway.GetVirtualGatewayId())

	if d.IsNewResource() {
		if err := setOSCAPITags(conn, d); err != nil {
			return err
		}
		d.SetPartial("tag")
	}

	return resourceOutscaleOAPIVirtualGatewayRead(d, meta)
}

func resourceOutscaleOAPIVirtualGatewayRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).OSCAPI

	var resp oscgo.ReadVirtualGatewaysResponse
	var err error

	err = resource.Retry(5*time.Minute, func() *resource.RetryError {
		resp, _, err = conn.VirtualGatewayApi.ReadVirtualGateways(context.Background()).ReadVirtualGatewaysRequest(oscgo.ReadVirtualGatewaysRequest{
			Filters: &oscgo.FiltersVirtualGateway{VirtualGatewayIds: &[]string{d.Id()}},
		}).Execute()
		if err != nil {
			if strings.Contains(err.Error(), "RequestLimitExceeded:") {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		return resource.NonRetryableError(err)
	})
	if err != nil {
		if strings.Contains(fmt.Sprint(err), "InvalidVpnGatewayID.NotFound") {
			d.SetId("")
			return nil
		}
		fmt.Printf("\n\n[ERROR] Error finding VpnGateway: %s", err)
		return err
	}

	if len(resp.GetVirtualGateways()) == 0 {
		return fmt.Errorf("[ERROR] Error finding VpnGateway: doesn't exists with id %s", d.Id())

	}

	virtualGateway := resp.GetVirtualGateways()[0]
	if virtualGateway.GetState() == "deleted" {
		d.SetId("")
		return nil
	}
	vpnLink := oapiVpnGatewayGetLink(virtualGateway)
	if len(virtualGateway.GetNetToVirtualGatewayLinks()) == 0 || vpnLink.GetState() == "detached" {
		d.Set("net_id", "")
	} else {
		d.Set("net_id", vpnLink.GetNetId())
	}

	vs := make([]map[string]interface{}, len(virtualGateway.GetNetToVirtualGatewayLinks()))

	for k, v := range virtualGateway.GetNetToVirtualGatewayLinks() {
		vp := make(map[string]interface{})
		vp["state"] = v.GetState()
		vp["net_id"] = v.GetNetId()

		vs[k] = vp
	}

	d.Set("connection_type", virtualGateway.GetConnectionType())
	d.Set("virtual_gateway_id", virtualGateway.GetVirtualGatewayId())
	d.Set("net_to_virtual_gateway_links", vs)
	d.Set("state", virtualGateway.State)
	d.Set("tags", tagsOSCAPIToMap(virtualGateway.GetTags()))

	return nil
}

func resourceOutscaleOAPIVirtualGatewayUpdate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).OSCAPI
	d.Partial(true)
	if err := setOSCAPITags(conn, d); err != nil {
		return err
	}
	d.SetPartial("tags")
	d.Partial(false)

	return nil
}

func resourceOutscaleOAPIVirtualGatewayDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).OSCAPI

	return resource.Retry(5*time.Minute, func() *resource.RetryError {
		_, _, err := conn.VirtualGatewayApi.DeleteVirtualGateway(context.Background()).DeleteVirtualGatewayRequest(oscgo.DeleteVirtualGatewayRequest{
			VirtualGatewayId: d.Id(),
		}).Execute()
		if err == nil {
			return nil
		}

		ec2err, ok := err.(awserr.Error)
		if !ok {
			return resource.RetryableError(err)
		}

		switch ec2err.Code() {
		case "InvalidVpnGatewayID.NotFound":
			return nil
		case "IncorrectState":
			return resource.RetryableError(err)
		}

		return resource.NonRetryableError(err)
	})
}

// vpnGatewayAttachStateRefreshFunc returns a resource.StateRefreshFunc that is used to watch
// the state of a VPN gateway's attachment
func vpnGatewayAttachStateRefreshFunc(conn *oscgo.APIClient, id string, expected string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		var resp oscgo.ReadVirtualGatewaysResponse
		var err error

		err = resource.Retry(5*time.Minute, func() *resource.RetryError {
			resp, _, err = conn.VirtualGatewayApi.ReadVirtualGateways(context.Background()).ReadVirtualGatewaysRequest(oscgo.ReadVirtualGatewaysRequest{
				Filters: &oscgo.FiltersVirtualGateway{VirtualGatewayIds: &[]string{id}},
			}).Execute()
			if err != nil {
				if strings.Contains(err.Error(), "RequestLimitExceeded:") {
					return resource.RetryableError(err)
				}
				return resource.NonRetryableError(err)
			}
			return resource.NonRetryableError(err)
		})

		if err != nil {
			if strings.Contains(fmt.Sprint(err), "InvalidVpnGatewayID.NotFound") {
				resp.SetVirtualGateways(nil)
			} else {
				fmt.Printf("[ERROR] Error on VpnGatewayStateRefresh: %s", err)
				return nil, "", err
			}
		}

		if resp.GetVirtualGateways() == nil {
			return nil, "", nil
		}

		virtualGateway := resp.GetVirtualGateways()[0]
		if len(virtualGateway.GetNetToVirtualGatewayLinks()) == 0 {
			return virtualGateway, "detached", nil
		}

		vpnAttachment := oapiVpnGatewayGetLink(virtualGateway)
		return virtualGateway, vpnAttachment.GetState(), nil
	}
}

func oapiVpnGatewayGetLink(vgw oscgo.VirtualGateway) *oscgo.NetToVirtualGatewayLink {
	for _, v := range vgw.GetNetToVirtualGatewayLinks() {
		if v.GetState() == "attached" {
			return &v
		}
	}
	return &oscgo.NetToVirtualGatewayLink{State: aws.String("detached")}
}

func virtualGatewayStateRefreshFunc(conn *oscgo.APIClient, instanceID, failState string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		resp, _, err := conn.VirtualGatewayApi.ReadVirtualGateways(context.Background()).ReadVirtualGatewaysRequest(oscgo.ReadVirtualGatewaysRequest{
			Filters: &oscgo.FiltersVirtualGateway{
				VirtualGatewayIds: &[]string{instanceID},
			},
		}).Execute()

		if err != nil {
			log.Printf("[ERROR] error on InstanceStateRefresh: %s", err)
			return nil, "", err
		}

		if !resp.HasVirtualGateways() {
			return nil, "", nil
		}

		virtualGateway := resp.GetVirtualGateways()[0]
		state := virtualGateway.GetState()

		if state == failState {
			return virtualGateway, state, fmt.Errorf("Failed to reach target state. Reason: %v", virtualGateway.State)

		}

		return virtualGateway, state, nil
	}
}
